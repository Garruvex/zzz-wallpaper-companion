package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func holidayRequest(s *APIServer, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	return w
}

const samplePublicHolidays = `[{"date":"2026-01-01","name":"New Year","countryCode":"US","nationalHoliday":true,"holidayTypes":["Public"]},{"date":"2026-04-03","name":"Good Friday","countryCode":"US","nationalHoliday":false,"subdivisionCodes":["US-CA"],"holidayTypes":["Public"]},{"date":"2026-04-05","name":"Observance","countryCode":"US","nationalHoliday":true,"holidayTypes":["Observance"]}]`

func TestHolidayFilteringPersistenceAndRegions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "Countries/Available") {
			fmt.Fprint(w, `[{"countryCode":"US","name":"United States"}]`)
			return
		}
		fmt.Fprint(w, samplePublicHolidays)
	}))
	defer upstream.Close()
	s := testServer(t)
	s.holidays.baseURL = upstream.URL
	for _, selection := range []struct {
		Body  string
		Count int
	}{{`{"country":"US","subdivision":""}`, 1}, {`{"country":"US","subdivision":"US-CA"}`, 2}} {
		if w := holidayRequest(s, "POST", "/api/v1/holiday-settings", selection.Body); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		w := holidayRequest(s, "GET", "/api/v1/holidays?year=2026", "")
		var d struct {
			Available bool
			Records   []HolidayRecord
		}
		if json.Unmarshal(w.Body.Bytes(), &d) != nil || !d.Available || len(d.Records) != selection.Count {
			t.Fatal(w.Body.String())
		}
	}
	w := holidayRequest(s, "GET", "/api/v1/holiday-subdivisions?country=US&year=2026", "")
	if !strings.Contains(w.Body.String(), "California") {
		t.Fatal(w.Body.String())
	}
	reloaded := newHolidayService(filepath.Dir(s.holidays.path))
	if reloaded.state.Settings.Subdivision != "US-CA" {
		t.Fatal("lost region")
	}
}
func TestHolidayCacheSurvivesInvalidUpdatesAndRestart(t *testing.T) {
	calls := 0
	broken := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if broken {
			fmt.Fprint(w, `[{"date":"2027-01-01","name":"Wrong year","countryCode":"US","holidayTypes":["Public"]}]`)
		} else {
			fmt.Fprint(w, samplePublicHolidays)
		}
	}))
	defer upstream.Close()
	h := newHolidayService(t.TempDir())
	h.baseURL = upstream.URL
	h.mu.Lock()
	records, stale, err := h.records(context.Background(), "US", 2026)
	h.mu.Unlock()
	if err != nil || stale || len(records) != 2 {
		t.Fatal(records, stale, err)
	}
	h.mu.Lock()
	_, _, _ = h.records(context.Background(), "US", 2026)
	h.mu.Unlock()
	if calls != 1 {
		t.Fatal("fresh cache fetched again")
	}
	cached := h.state.Cache["US-2026"]
	cached.Updated = time.Now().Add(-8 * 24 * time.Hour)
	h.state.Cache["US-2026"] = cached
	h.attempted = map[string]time.Time{}
	_ = h.persist()
	broken = true
	h = newHolidayService(filepath.Dir(h.path))
	h.baseURL = upstream.URL
	h.mu.Lock()
	records, stale, err = h.records(context.Background(), "US", 2026)
	h.mu.Unlock()
	if err != nil || !stale || len(records) != 2 {
		t.Fatal(records, stale, err)
	}
	h.mu.Lock()
	_, _, _ = h.records(context.Background(), "US", 2026)
	h.mu.Unlock()
	if calls != 2 {
		t.Fatal("failed source was not backed off")
	}
}
func TestHolidaySettingsValidationAndCorruptFile(t *testing.T) {
	s := testServer(t)
	for _, body := range []string{`{"country":"../US","subdivision":""}`, `{"country":"US","subdivision":"CA-ON"}`, `{"country":"TW","subdivision":"TW-TPE"}`, `{"country":"","subdivision":""} {}`} {
		if w := holidayRequest(s, "POST", "/api/v1/holiday-settings", body); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, year := range []string{"", "1800", "2201", "2026/US"} {
		if w := holidayRequest(s, "GET", "/api/v1/holidays?year="+year, ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	_ = os.WriteFile(s.holidays.path, []byte("broken"), 0600)
	s.holidays = newHolidayService(filepath.Dir(s.holidays.path))
	if w := holidayRequest(s, "POST", "/api/v1/holiday-settings", `{"country":"","subdivision":""}`); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if err := s.holidays.persist(); err == nil {
		t.Fatal("overwrote corrupt state")
	}
	b, _ := os.ReadFile(s.holidays.path)
	if string(b) != "broken" {
		t.Fatal("lost corrupt file")
	}
}
func TestChinaHolidayArrangementsAndUnpublishedYears(t *testing.T) {
	records, err := parseChina([]byte(`{"year":2026,"papers":["https://www.gov.cn/notice"],"days":[{"date":"2026-10-01","name":"国庆节","isOffDay":true},{"date":"2026-10-10","name":"国庆节","isOffDay":false}]}`), 2026)
	if err != nil || records[0].Kind != "observed" || records[1].Kind != "workday" {
		t.Fatal(records, err)
	}
	for _, raw := range []string{`{"year":2027,"papers":[],"days":[]}`, `{"year":2026,"papers":["x"],"days":[{"date":"2026-02-29","name":"x","isOffDay":true}]}`, `{"year":2026,"papers":["x"],"days":[{"date":"2026-01-01","name":"x"}]}`} {
		if _, err := parseChina([]byte(raw), 2026); err == nil {
			t.Fatal("accepted invalid/unpublished calendar")
		}
	}
}
func TestTaiwanFullCalendarExcludesOrdinaryWeekends(t *testing.T) {
	var b strings.Builder
	b.WriteString("\ufeff西元日期,星期,是否放假,備註\n")
	for date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); date.Year() == 2026; date = date.AddDate(0, 0, 1) {
		off := "0"
		note := ""
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			off = "2"
		}
		if date.Month() == 1 && date.Day() == 1 {
			off = "2"
			note = "開國紀念日"
		}
		if date.Month() == 1 && date.Day() == 3 {
			off = "0"
			note = "補行上班"
		}
		if date.Month() == 1 && date.Day() == 2 {
			off = "2"
			note = "調整放假"
		}
		fmt.Fprintf(&b, "%s,%d,%s,%s\n", date.Format("20060102"), date.Weekday(), off, note)
	}
	records, err := parseTaiwan([]byte(b.String()), 2026)
	if err != nil || len(records) != 3 || records[1].Kind != "observed" || records[2].Kind != "workday" {
		t.Fatal(records, err)
	}
	if _, err := parseTaiwan([]byte("西元日期,是否放假,備註\n20260101,2,元旦\n"), 2026); err == nil {
		t.Fatal("accepted partial calendar")
	}
}

// Opt-in smoke test against real providers; the regular test suite stays offline.
func TestHolidayLiveSources(t *testing.T) {
	if os.Getenv("ZZZ_HOLIDAY_LIVE_TEST") != "1" {
		t.Skip("set ZZZ_HOLIDAY_LIVE_TEST=1 to check public sources")
	}
	h := newHolidayService(t.TempDir())
	h.mu.Lock()
	countries, _, err := h.countries(context.Background())
	h.mu.Unlock()
	if err != nil || len(countries) < 50 {
		t.Fatalf("country list: %d, %v", len(countries), err)
	}
	for _, country := range []string{"US", "CN", "TW"} {
		t.Run(country, func(t *testing.T) {
			h.mu.Lock()
			records, _, err := h.records(context.Background(), country, 2026)
			h.mu.Unlock()
			if err != nil || len(records) == 0 {
				t.Fatalf("%s: %d records, %v", country, len(records), err)
			}
			t.Logf("%s: %d valid records", country, len(records))
		})
	}
}
