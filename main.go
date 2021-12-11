package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type House struct {
	gorm.Model
	Id      string
	Link    string
	Title   string
	Type    string
	Layout  string
	Size    string
	Floor   string
	Area    string
	Address string
	Price   string
}

func main() {
	initEnv()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
	)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	//ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	err := chromedp.Run(ctx, chromedp.Navigate(os.Getenv("591_SEARCH_URL")))
	if err != nil {
		log.Fatal(err)
	}
	for {
		var rentItems []*cdp.Node
		err = chromedp.Run(ctx,
			chromedp.WaitVisible("#rent-list-app"),
			chromedp.Nodes("section.vue-list-rent-item", &rentItems),
		)
		if err != nil {
			log.Fatal(err)
		}
		db := initDb()

		var newLinks []string
		for _, rentItem := range rentItems {
			var house House
			house.Id = rentItem.AttributeValue("data-bind")
			err = chromedp.Run(ctx, chromedp.Text("div.item-title", &house.Title, chromedp.ByQuery, chromedp.FromNode(rentItem)))
			if err != nil {
				log.Fatal("Failed to retrieve item title")
			}

			var ok bool
			err = chromedp.Run(ctx, chromedp.AttributeValue("a", "href", &house.Link, &ok, chromedp.ByQuery, chromedp.FromNode(rentItem)))
			if err != nil || !ok {
				log.Fatal("Failed to retrieve item link")
			}

			var itemStyles []*cdp.Node
			err = chromedp.Run(ctx, chromedp.Nodes("ul.item-style > li", &itemStyles, chromedp.ByQueryAll, chromedp.FromNode(rentItem)))
			if err != nil {
				log.Fatal("Failed to retrieve item style")
			}
			house.Type = itemStyles[0].Children[0].NodeValue
			house.Layout = itemStyles[1].Children[0].NodeValue
			house.Size = itemStyles[2].Children[0].NodeValue
			house.Floor = itemStyles[3].Children[0].NodeValue

			err = chromedp.Run(ctx, chromedp.Text("div.item-area > a", &house.Area, chromedp.ByQuery, chromedp.FromNode(rentItem)))
			if err != nil {
				log.Fatal("Failed to retrieve item area")
			}

			err = chromedp.Run(ctx, chromedp.Text("div.item-area > span", &house.Address, chromedp.ByQuery, chromedp.FromNode(rentItem)))
			if err != nil {
				log.Fatal("Failed to retrieve item address")
			}

			err = chromedp.Run(ctx, chromedp.Text("div.item-price", &house.Price, chromedp.ByQuery, chromedp.FromNode(rentItem)))
			if err != nil {
				log.Fatal("Failed to retrieve item price")
			}
			if db.Create(&house).Error != nil {
				continue
			}
			newLinks = append(newLinks, house.Link)
		}

		sendToDiscord(newLinks)

		time.Sleep(5 * time.Minute)
		err = chromedp.Run(ctx, chromedp.Reload())
		if err != nil {
			log.Fatal(err)
		}
	}
}

func initEnv() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal(err)
	}
}

func initDb() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatal(err)
	}
	if db.AutoMigrate(&House{}) != nil {
		log.Fatal(err)
	}
	return db
}

func sendToDiscord(links []string) *http.Response {
	body := map[string]string{"content": strings.Join(links, "\n\n")}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		log.Fatal("Failed to encode request body", err)
	}
	resp, err := http.Post(
		os.Getenv("DISCORD_WEBHOOK_URL"),
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		log.Fatal(err)
	}

	return resp
}
