package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"net/http"
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

var subscriptions []map[string]string

func init() {
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/config")
	viper.SetDefault("dbDsn", "test.db")
	viper.SetDefault("refreshIntervalMinutes", 10)
	viper.SetDefault("debug", false)
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}
	err = viper.UnmarshalKey("subscriptions", &subscriptions)
	if err != nil {
		log.Fatal(err)
	}
	viper.WatchConfig()
}

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !viper.GetBool("debug")),
	)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	//ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	for {
		for _, subscription := range subscriptions {
			subscriptionName := subscription["name"]
			log.Printf("Start retrieving new houses for subscription '%s' ...", subscriptionName)

			err := chromedp.Run(ctx, chromedp.Navigate(subscription["searchUrl"]))
			if err != nil {
				log.Fatal(err)
			}

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
				var dupHouse House
				result := db.First(
					&dupHouse,
					"id != ? AND type = ? AND layout = ? AND floor = ? AND area = ? AND address = ?",
					house.Id, house.Type, house.Layout, house.Floor, house.Area, house.Address,
				)
				if result.Error != gorm.ErrRecordNotFound {
					db.Unscoped().Delete(&dupHouse)
					db.Create(&house)
				} else {
					if db.Create(&house).Error == nil {
						newLinks = append(newLinks, house.Link)
					}
				}
			}

			if len(newLinks) != 0 {
				sendToDiscord(subscriptionName, subscription["discordWebhookUrl"], newLinks)
			}
		}

		sleepDuration := viper.GetDuration("refreshIntervalMinutes")
		log.Printf("Round finished, sleeping for %d minutes...", sleepDuration)
		time.Sleep(sleepDuration * time.Minute)
	}
}

func initDb() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(viper.GetString("dbDsn")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatal(err)
	}
	if db.AutoMigrate(&House{}) != nil {
		log.Fatal(err)
	}
	return db
}

func sendToDiscord(name string, webhookUrl string, links []string) *http.Response {
	log.Printf("Sending %d new houses to discord...", len(links))
	body := map[string]string{"content": "Subscription name: " + name + "\n\n" + strings.Join(links, "\n\n")}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		log.Fatal("Failed to encode request body", err)
	}
	resp, err := http.Post(
		webhookUrl,
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		log.Fatal(err)
	}

	return resp
}
