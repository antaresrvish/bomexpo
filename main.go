package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Product struct {
	Context     string `json:"@context"`
	Type        string `json:"@type"`
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	MPN         string `json:"mpn"`
	Brand       string `json:"brand"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Category    string `json:"category"`
	Offers      Offer  `json:"offers"`
}

type Offer struct {
	Type           string `json:"@type"`
	URL            string `json:"url"`
	PriceCurrency  string `json:"priceCurrency"`
	Price          string `json:"price"`
	ItemCondition  string `json:"itemCondition"`
	Availability   string `json:"availability"`
	InventoryLevel int    `json:"inventoryLevel"`
	Seller         Seller `json:"seller"`
	Description    string `json:"description"`
}

type Seller struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

func main() {
	var product Product
	var cursor int
	var lcscPart string

	const startTag string = `<script data-n-head="ssr" type="application/ld+json">`
	const endTag string = `</script>`

	println("LCSC Part: ")
	fmt.Scanf("%v", &lcscPart)

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", "https://www.lcsc.com/product-detail/"+lcscPart+".html", nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header = map[string][]string{
		"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"},
		"Accept":          {"ext/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		"Accept-Language": {"en-US"},
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	body := string(content)
	cursor = 0
	for {
		body = body[cursor:]

		start := strings.Index(body, startTag)
		if start == -1 {
			break
		}
		fragment := body[start:]
		end := strings.Index(fragment, endTag)
		if end == -1 {
			break
		}
		cursor = end
		fragment = fragment[:end]
		fragment = strings.TrimLeft(fragment, startTag)
		err = json.Unmarshal([]byte(fragment), &product)
		if product.Type == "Product" {
			cursor = 0
			prettyJson, err := json.MarshalIndent(product, "", "   ")
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("%s\n", prettyJson)
			break
		}
		continue

	}
}
