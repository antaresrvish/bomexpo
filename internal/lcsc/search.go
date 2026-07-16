package lcsc

import "net/url"

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

func (c *Client) Detail(code string) (Part, error) {
	var p Part
	err := c.call("GET", baseWMSC+"/ftps/wm/product/detail?productCode="+url.QueryEscape(code), nil, &p)
	return p, err
}

func (c *Client) CategoryTree() ([]Category, error) {
	var cats []Category
	err := c.call("GET", baseWWW+"/bff/cache?path=%2Fftps%2Fwm%2Fhome%2Fcategory&duration=1800000", nil, &cats)
	return cats, err
}
