package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 한국천문연구원 특일 정보. getRestDeInfo 만 쓴다.
// getHoliDeInfo(국경일)는 제헌절처럼 쉬지 않는 날도 포함하므로 쓰지 않는다. §23.13.2
const holidayAPIBase = "https://apis.data.go.kr/B090041/openapi/service/SpcdeInfoService"

// HOLIDAY_API_KEY 는 공공데이터포털 Decoding 키(원문, + 와 == 포함)를 넣는다. §23.13.2 방법 A
// url.Values.Encode() 가 serviceKey 를 한 번만 인코딩한다.
// Encoding 키(%2B…%3D%3D)를 넣으면 아래에서 한 번만 디코드한다. Encode() 된 키를 그대로 Encode() 하면
// %2B → %252B 가 되어 SERVICE_KEY_IS_NOT_REGISTERED_ERROR 가 난다.

type HolidayAPI interface {
	FetchRestDe(year, month int) ([]model.Holiday, error)
}

type HolidayAPIClient struct {
	HTTP    *http.Client
	BaseURL string
	Key     string
}

func NewHolidayAPIClient() *HolidayAPIClient {
	return &HolidayAPIClient{
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		BaseURL: holidayAPIBase,
	}
}

func (c *HolidayAPIClient) serviceKey() string {
	if c != nil && strings.TrimSpace(c.Key) != "" {
		return normalizeHolidayAPIKey(c.Key)
	}
	return normalizeHolidayAPIKey(os.Getenv("HOLIDAY_API_KEY"))
}

// normalizeHolidayAPIKey Decoding 키는 그대로, Encoding 키(%…)만 한 번 디코드한다. 방법 A.
func normalizeHolidayAPIKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Decoding 키의 + 를 QueryUnescape 하면 공백이 되므로, % 가 있을 때만 디코드한다.
	if strings.Contains(raw, "%") {
		if dec, err := url.QueryUnescape(raw); err == nil && dec != "" {
			return dec
		}
	}
	return raw
}

func buildRestDeURL(base, key string, year, month int) (string, error) {
	key = normalizeHolidayAPIKey(key)
	if key == "" {
		return "", fmt.Errorf("HOLIDAY_API_KEY 가 없습니다")
	}
	if year < 2000 || year > 2100 || month < 1 || month > 12 {
		return "", fmt.Errorf("연·월이 올바르지 않습니다")
	}
	if strings.TrimSpace(base) == "" {
		base = holidayAPIBase
	}
	q := url.Values{}
	q.Set("serviceKey", key)
	q.Set("solYear", fmt.Sprintf("%d", year))
	q.Set("solMonth", fmt.Sprintf("%02d", month))
	q.Set("numOfRows", "100")
	q.Set("_type", "json")
	return strings.TrimRight(base, "/") + "/getRestDeInfo?" + q.Encode(), nil
}

func (c *HolidayAPIClient) FetchRestDe(year, month int) ([]model.Holiday, error) {
	if c == nil {
		c = NewHolidayAPIClient()
	}
	u, err := buildRestDeURL(c.BaseURL, c.serviceKey(), year, month)
	if err != nil {
		return nil, err
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("특일 API HTTP %d", resp.StatusCode)
	}
	return parseRestDeJSON(body, year)
}

type restDeEnvelope struct {
	Response struct {
		Header struct {
			ResultCode string `json:"resultCode"`
			ResultMsg  string `json:"resultMsg"`
		} `json:"header"`
		Body struct {
			Items json.RawMessage `json:"items"`
		} `json:"body"`
	} `json:"response"`
}

type restDeItem struct {
	DateName  string          `json:"dateName"`
	IsHoliday string          `json:"isHoliday"`
	Locdate   json.RawMessage `json:"locdate"`
}

func parseRestDeJSON(raw []byte, year int) ([]model.Holiday, error) {
	var env restDeEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("특일 API JSON: %w", err)
	}
	code := strings.TrimSpace(env.Response.Header.ResultCode)
	if code != "" && code != "00" && code != "0000" {
		msg := strings.TrimSpace(env.Response.Header.ResultMsg)
		if msg == "" {
			msg = code
		}
		return nil, fmt.Errorf("특일 API: %s", msg)
	}
	items, err := parseRestDeItems(env.Response.Body.Items)
	if err != nil {
		return nil, err
	}
	out := make([]model.Holiday, 0, len(items))
	for _, it := range items {
		if !strings.EqualFold(strings.TrimSpace(it.IsHoliday), "Y") {
			continue
		}
		ds, ok := parseLocdate(it.Locdate)
		if !ok {
			continue
		}
		name := strings.TrimSpace(it.DateName)
		if name == "" {
			name = "공휴일"
		}
		kind := model.HolidayKindPublic
		if strings.Contains(name, "대체") {
			kind = model.HolidayKindSubstitute
		}
		out = append(out, model.Holiday{
			Date: ds, Year: year, Name: name, Kind: kind, Source: model.HolidaySourceAPI,
		})
	}
	return out, nil
}

func parseRestDeItems(raw json.RawMessage) ([]restDeItem, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "\"\"" || s == "null" {
		return nil, nil
	}
	var wrap struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, nil
	}
	itemRaw := strings.TrimSpace(string(wrap.Item))
	if itemRaw == "" || itemRaw == "null" {
		return nil, nil
	}
	if strings.HasPrefix(itemRaw, "[") {
		var items []restDeItem
		if err := json.Unmarshal(wrap.Item, &items); err != nil {
			return nil, fmt.Errorf("특일 API item: %w", err)
		}
		return items, nil
	}
	var one restDeItem
	if err := json.Unmarshal(wrap.Item, &one); err != nil {
		return nil, fmt.Errorf("특일 API item: %w", err)
	}
	return []restDeItem{one}, nil
}

func parseLocdate(raw json.RawMessage) (string, bool) {
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	if len(s) != 8 {
		return "", false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return "", false
		}
	}
	return s[0:4] + "-" + s[4:6] + "-" + s[6:8], true
}
