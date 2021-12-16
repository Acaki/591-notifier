package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/fsnotify/fsnotify"
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

var subscriptions []map[string]interface{}
var currentDsn string
var db *gorm.DB

func init() {
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/config")
	viper.SetDefault("dbDsn", "test.db")
	viper.SetDefault("refreshIntervalMinutes", 10)
	viper.SetDefault("debug", false)
	viper.SetDefault("navigationTimeout", 60)
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}
	db = initDb(viper.GetString("dbDsn"))
	loadSubscriptions()
	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Println("Config file changed:", e.Name)
		loadSubscriptions()
		reloadDb(db)
	})
	viper.WatchConfig()
}

func reloadDb(db *gorm.DB) {
	newDsn := viper.GetString("dbDsn")
	if newDsn != currentDsn {
		sqlDB, err := db.DB()
		if err != nil {
			log.Fatal("Failed to get db instance")
		}
		err = sqlDB.Close()
		if err != nil {
			log.Fatal("Failed to close db connection")
		}
		db = initDb(newDsn)
	}
}

func loadSubscriptions() {
	err := viper.UnmarshalKey("subscriptions", &subscriptions)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !viper.GetBool("debug")),
	)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	for {
		for _, subscription := range subscriptions {
			subscriptionName := fmt.Sprint(subscription["name"])
			searchUrl := fmt.Sprint(subscription["searchUrl"])
			discordWebhookUrl := fmt.Sprint(subscription["discordWebhookUrl"])
			var ruleOutSingleBathroom bool
			flag, ok := subscription["ruleOutSingleBathroom"]
			if ok && flag == true {
				ruleOutSingleBathroom = true
			} else {
				ruleOutSingleBathroom = false
			}

			log.Printf("Start retrieving new houses for subscription '%s' ...", subscriptionName)
			newLinks := getNewLinks(ctx, db, searchUrl, ruleOutSingleBathroom)

			if len(newLinks) != 0 {
				sendToDiscord(subscriptionName, discordWebhookUrl, newLinks)
			}
		}

		sleepDuration := viper.GetDuration("refreshIntervalMinutes")
		log.Printf("Round finished, sleeping for %d minutes...", sleepDuration)
		time.Sleep(sleepDuration * time.Minute)
	}
}

func getNewLinks(ctx context.Context, db *gorm.DB, searchUrl string, ruleOutSingleBathroom bool) []string {
	ctx, cancel := context.WithTimeout(ctx, viper.GetDuration("navigationTimeout")*time.Second)
	defer cancel()
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	var newLinks []string
	var rentItems []*cdp.Node
	err := chromedp.Run(ctx,
		chromedp.Navigate(searchUrl),
		chromedp.WaitVisible("#rent-list-app"),
		chromedp.Nodes("section.vue-list-rent-item", &rentItems),
	)
	if err != nil {
		log.Println(err)
		if err == context.DeadlineExceeded {
			return newLinks
		}
	}

	for _, rentItem := range rentItems {
		var house House

		var ok bool
		err = chromedp.Run(ctx, chromedp.AttributeValue("a", "href", &house.Link, &ok, chromedp.ByQuery, chromedp.FromNode(rentItem), chromedp.AtLeast(0)))
		if err != nil || !ok {
			log.Println("Failed to retrieve item link")
			if err == context.DeadlineExceeded {
				break
			}
		}

		var layout string
		if ruleOutSingleBathroom {
			newTab, cancel := context.WithTimeout(ctx, viper.GetDuration("navigationTimeout")*time.Second)
			newTab, _ = chromedp.NewContext(newTab)
			err := chromedp.Run(newTab, chromedp.Navigate(house.Link), chromedp.WaitVisible("#houseInfo"), chromedp.Text("#houseInfo > div.house-pattern > span", &layout))
			if err != nil {
				log.Println("Failed to retrieve item layout in detailed link")
			}
			if err == context.DeadlineExceeded || strings.Contains(layout, "1衛") {
				cancel()
				continue
			}
			cancel()
		}

		house.Id = rentItem.AttributeValue("data-bind")
		err = chromedp.Run(ctx, chromedp.Text("div.item-title", &house.Title, chromedp.ByQuery, chromedp.FromNode(rentItem), chromedp.AtLeast(0)))
		if err != nil {
			log.Println("Failed to retrieve item title")
			if err == context.DeadlineExceeded {
				break
			}
		}

		var itemStyles []*cdp.Node
		err = chromedp.Run(ctx, chromedp.Nodes("ul.item-style > li", &itemStyles, chromedp.ByQueryAll, chromedp.FromNode(rentItem), chromedp.AtLeast(0)))
		if err != nil {
			log.Println("Failed to retrieve item style")
			if err == context.DeadlineExceeded {
				break
			}
		}
		house.Type = itemStyles[0].Children[0].NodeValue
		if layout != "" {
			house.Layout = layout
		} else {
			house.Layout = itemStyles[1].Children[0].NodeValue
		}
		house.Size = itemStyles[2].Children[0].NodeValue
		house.Floor = itemStyles[3].Children[0].NodeValue

		err = chromedp.Run(ctx, chromedp.Text("div.item-area > a", &house.Area, chromedp.ByQuery, chromedp.FromNode(rentItem), chromedp.AtLeast(0)))
		if err != nil {
			log.Println("Failed to retrieve item area")
			if err == context.DeadlineExceeded {
				break
			}
		}

		err = chromedp.Run(ctx, chromedp.Text("div.item-area > span", &house.Address, chromedp.ByQuery, chromedp.FromNode(rentItem), chromedp.AtLeast(0)))
		if err != nil {
			log.Println("Failed to retrieve item address")
			if err == context.DeadlineExceeded {
				break
			}
		}

		err = chromedp.Run(ctx, chromedp.Text("div.item-price", &house.Price, chromedp.ByQuery, chromedp.FromNode(rentItem), chromedp.AtLeast(0)))
		if err != nil {
			log.Println("Failed to retrieve item price")
			if err == context.DeadlineExceeded {
				break
			}
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
	return newLinks
}

func initDb(dsn string) *gorm.DB {
	currentDsn = dsn
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
		log.Println(err)
	}

	return resp
}
