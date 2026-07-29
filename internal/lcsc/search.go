package lcsc

import (
	"encoding/json"
	"net/url"
)

type searchReq struct {
	Keyword     string `json:"keyword"`
	CurrentPage int    `json:"currentPage"`
	PageSize    int    `json:"pageSize"`
}

type searchResp struct {
	CurrPage int    `json:"currPage"`
	PageRow  int    `json:"pageRow"`
	TotalRow int    `json:"totalRow"`
	DataList []Part `json:"dataList"`
}

func (c *Client) Search(keyword string, page, size int) (SearchResult, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 25
	}
	var r searchResp
	err := c.call("POST", baseWMSC+"/ftps/wm/product/query/list",
		searchReq{Keyword: keyword, CurrentPage: page, PageSize: size}, &r)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Items: r.DataList, Total: r.TotalRow, Page: page, PageSize: size}, nil
}

// Detail returns a part, served from the on-disk cache when a fresh copy
// exists and falling back to a stale copy if the network is unavailable.
func (c *Client) Detail(code string) (Part, error) {
	return c.detail(code, false)
}

// Refresh re-fetches a part from LCSC, bypassing the cache, and stores the
// fresh stock and pricing back into it.
func (c *Client) Refresh(code string) (Part, error) {
	return c.detail(code, true)
}

func (c *Client) detail(code string, force bool) (Part, error) {
	raw, fresh, ok := c.w.CacheGet(code)
	if ok && fresh && !force {
		var p Part
		if json.Unmarshal(raw, &p) == nil {
			return p, nil
		}
	}
	fetched, err := c.callRaw("GET", baseWMSC+"/ftps/wm/product/detail?productCode="+url.QueryEscape(code), nil)
	if err != nil {
		if ok && !force { // offline: serve whatever we cached, even if stale
			var p Part
			if json.Unmarshal(raw, &p) == nil {
				return p, nil
			}
		}
		return Part{}, err
	}
	c.w.CachePut(code, fetched)
	var p Part
	if len(fetched) > 0 {
		err = json.Unmarshal(fetched, &p)
	}
	return p, err
}

func (c *Client) CategoryTree() ([]Category, error) {
	var cats []Category
	err := c.call("GET", baseWWW+"/bff/cache?path=%2Fftps%2Fwm%2Fhome%2Fcategory&duration=1800000", nil, &cats)
	return cats, err
}
