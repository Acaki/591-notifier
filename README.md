# 591-notifier

Get notified in Discord when new houses are posted in https://www.591.com.tw/

## Configuration

```json5
{
  // Disable headless mode of the browser
  "debug": false,
  // SQLite store path
  "dbDsn": "test.db",
  // Refresh interval in minutes to fetch for new data
  "refreshIntervalMinutes": 5,
  // Timeout in seconds when fetching search result page
  "fetchSubscriptionTimeout": 120,
  // Timeout in seconds when fetching house detail page
  "fetchHouseDetailTimeout": 10,
  "subscriptions": [
    {
      // Name of the subscription in discord message
      "name": "Your subscription name",
      // Search url, use the following for example
      "searchUrl": "https://rent.591.com.tw/?region=1&kind=1&showMore=1&order=posttime&orderType=desc",
      // Rule out new houses that only has single bathroom
      "ruleOutSingleBathroom": false,
      // Discord webhook url for the channel you want to receive notifications
      "discordWebhookUrl": "https://discord.com/api/webhooks/<id>/<token>"
    }
  ]
}
```

## Build

* Simply build with `go build` and run it
* Use the provided `Dockerfile`, make sure to mount your database file as volume