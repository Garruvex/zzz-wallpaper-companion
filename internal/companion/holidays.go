package companion

import (
	"context"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const holidayRefresh = 7 * 24 * time.Hour

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
var subdivisionPattern = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,6}$`)

type HolidaySettings struct {
	Country     string `json:"country"`
	Subdivision string `json:"subdivision"`
}
type HolidayRecord struct {
	Date         string   `json:"date"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	National     bool     `json:"national"`
	Subdivisions []string `json:"subdivisions,omitempty"`
}
type holidayCache struct {
	Updated time.Time `json:"updated"`
	Data    []byte    `json:"data"`
}
type holidayState struct {
	Settings HolidaySettings         `json:"settings"`
	Cache    map[string]holidayCache `json:"cache"`
}
type holidayService struct {
	mu        sync.Mutex
	path      string
	state     holidayState
	client    *http.Client
	baseURL   string
	cnURL     string
	twURL     string
	attempted map[string]time.Time
	loadError error
}
type holidayCountry struct {
	Code string `json:"countryCode"`
	Name string `json:"name"`
}
type holidaySubdivision struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Names from Unicode CLDR common/subdivisions/en.xml; see UNICODE_LICENSE.txt.
//
//go:embed subdivision_names.json
var subdivisionNamesJSON []byte

//go:embed UNICODE_LICENSE.txt
var unicodeLicense string

func (s *APIServer) unicodeLicensePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, unicodeLicense)
}

var subdivisionNames = func() map[string]string {
	var names map[string]string
	_ = json.Unmarshal(subdivisionNamesJSON, &names)
	return names
}()

func newHolidayService(dataDir string) *holidayService {
	s := &holidayService{path: filepath.Join(dataDir, "holidays.json"), client: &http.Client{Timeout: 12 * time.Second}, baseURL: "https://date.nager.at/api/v4", cnURL: "https://raw.githubusercontent.com/NateScarlet/holiday-cn/master", twURL: "https://data.gov.tw/api/v2/rest/dataset/14718", attempted: map[string]time.Time{}, state: holidayState{Cache: map[string]holidayCache{}}}
	b, err := os.ReadFile(s.path)
	if !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = json.Unmarshal(b, &s.state)
		}
		if err == nil {
			err = validateHolidaySettings(s.state.Settings)
		}
		s.loadError = err
	}
	if s.state.Cache == nil {
		s.state.Cache = map[string]holidayCache{}
	}
	return s
}
func validateHolidaySettings(v HolidaySettings) error {
	if v.Country == "" && v.Subdivision == "" {
		return nil
	}
	if !countryCodePattern.MatchString(v.Country) || (v.Subdivision != "" && (!subdivisionPattern.MatchString(v.Subdivision) || !strings.HasPrefix(v.Subdivision, v.Country+"-"))) {
		return errors.New("Choose a valid country and region.")
	}
	if (v.Country == "TW" || v.Country == "CN") && v.Subdivision != "" {
		return errors.New("This calendar uses national holiday arrangements.")
	}
	return nil
}
func (s *holidayService) persist() error {
	if s.loadError != nil {
		return s.loadError
	}
	b, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), "holidays-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.path)
	}
	return err
}
func (s *holidayService) fetch(ctx context.Context, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZZZWallpaperCompanion/"+version)
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("Holiday source returned %d.", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20+1))
	if len(body) > 4<<20 {
		return nil, errors.New("Holiday response is too large.")
	}
	return body, err
}

// The mutex is held by callers. Cache only validated responses; failures keep the last good copy.
func (s *holidayService) resource(ctx context.Context, key, address string, validate func([]byte) error) ([]byte, bool, error) {
	cached, ok := s.state.Cache[key]
	if ok && validate(cached.Data) != nil {
		ok = false
	}
	if ok && time.Since(cached.Updated) < holidayRefresh {
		return cached.Data, false, nil
	}
	if last := s.attempted[key]; !last.IsZero() && time.Since(last) < 15*time.Minute {
		if ok {
			return cached.Data, true, nil
		}
		return nil, false, errors.New("Holiday data is unavailable. Try again later.")
	}
	s.attempted[key] = time.Now()
	body, err := s.fetch(ctx, address)
	if err == nil {
		err = validate(body)
	}
	if err != nil {
		if ok {
			return cached.Data, true, nil
		}
		return nil, false, err
	}
	s.state.Cache[key] = holidayCache{Updated: time.Now(), Data: body}
	if len(s.state.Cache) > 64 {
		oldest := ""
		for k, v := range s.state.Cache {
			if k != key && (oldest == "" || v.Updated.Before(s.state.Cache[oldest].Updated)) {
				oldest = k
			}
		}
		delete(s.state.Cache, oldest)
	}
	_ = s.persist()
	return body, false, nil
}
func validateCountries(b []byte) error {
	var items []holidayCountry
	if json.Unmarshal(b, &items) != nil || len(items) == 0 || len(items) > 300 {
		return errors.New("Invalid country list.")
	}
	for _, v := range items {
		if !countryCodePattern.MatchString(v.Code) || v.Name == "" {
			return errors.New("Invalid country list.")
		}
	}
	return nil
}
func (s *holidayService) countries(ctx context.Context) ([]holidayCountry, bool, error) {
	b, stale, err := s.resource(ctx, "countries", s.baseURL+"/Countries/Available", validateCountries)
	items := []holidayCountry{}
	if err == nil {
		_ = json.Unmarshal(b, &items)
	}
	for _, v := range []holidayCountry{{"TW", "Taiwan"}, {"CN", "China"}} {
		found := false
		for _, item := range items {
			if item.Code == v.Code {
				found = true
			}
		}
		if !found {
			items = append(items, v)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, stale, err
}
func validHolidayDate(value string, year int) bool {
	d, err := time.Parse("2006-01-02", value)
	return err == nil && d.Year() == year
}
func validateRecords(records []HolidayRecord, year int) error {
	if len(records) > 800 {
		return errors.New("Too many holidays.")
	}
	for _, v := range records {
		if !validHolidayDate(v.Date, year) || strings.TrimSpace(v.Name) == "" || len(v.Name) > 500 || (v.Kind != "holiday" && v.Kind != "observed" && v.Kind != "workday") {
			return errors.New("Invalid holiday data.")
		}
		for _, code := range v.Subdivisions {
			if !subdivisionPattern.MatchString(code) {
				return errors.New("Invalid holiday region.")
			}
		}
	}
	return nil
}
func parseNager(b []byte, country string, year int) ([]HolidayRecord, error) {
	var items []struct {
		Date             string
		Name             string
		CountryCode      string
		NationalHoliday  bool
		SubdivisionCodes []string
		HolidayTypes     []string
	}
	if json.Unmarshal(b, &items) != nil || len(items) == 0 || len(items) > 800 {
		return nil, errors.New("Invalid holiday data.")
	}
	records := []HolidayRecord{}
	for _, item := range items {
		if item.CountryCode != country || !validHolidayDate(item.Date, year) || strings.TrimSpace(item.Name) == "" {
			return nil, errors.New("Invalid holiday data.")
		}
		public := false
		for _, kind := range item.HolidayTypes {
			if kind == "Public" {
				public = true
			}
		}
		if !public {
			continue
		}
		records = append(records, HolidayRecord{Date: item.Date, Name: item.Name, Kind: "holiday", National: item.NationalHoliday, Subdivisions: item.SubdivisionCodes})
	}
	return records, validateRecords(records, year)
}
func parseChina(b []byte, year int) ([]HolidayRecord, error) {
	var d struct {
		Year   int
		Papers []string
		Days   []struct {
			Date     string
			Name     string
			IsOffDay *bool
		}
	}
	if json.Unmarshal(b, &d) != nil || d.Year != year || len(d.Papers) == 0 || len(d.Days) == 0 || len(d.Days) > 800 {
		return nil, errors.New("This year's holiday arrangements have not been published.")
	}
	records := []HolidayRecord{}
	for _, day := range d.Days {
		if day.IsOffDay == nil {
			return nil, errors.New("Invalid holiday data.")
		}
		if !validHolidayDate(day.Date, year) {
			return nil, errors.New("Invalid holiday date.")
		}
		kind := "observed"
		if !*day.IsOffDay {
			kind = "workday"
		}
		records = append(records, HolidayRecord{Date: day.Date, Name: day.Name, Kind: kind, National: true})
	}
	return records, validateRecords(records, year)
}
func parseTaiwan(b []byte, year int) ([]HolidayRecord, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\ufeff")))
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 366 || len(rows) > 367 {
		return nil, errors.New("Incomplete Taiwan calendar.")
	}
	cols := map[string]int{}
	for i, v := range rows[0] {
		cols[strings.TrimSpace(v)] = i
	}
	for _, key := range []string{"西元日期", "是否放假", "備註"} {
		if _, ok := cols[key]; !ok {
			return nil, errors.New("Invalid Taiwan calendar columns.")
		}
	}
	records := []HolidayRecord{}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		day, err := time.Parse("20060102", strings.TrimSpace(row[cols["西元日期"]]))
		if err != nil || day.Year() != year || seen[day.Format("2006-01-02")] {
			return nil, errors.New("Invalid Taiwan calendar date.")
		}
		date := day.Format("2006-01-02")
		seen[date] = true
		off := strings.TrimSpace(row[cols["是否放假"]])
		if off != "0" && off != "2" {
			return nil, errors.New("Invalid Taiwan calendar day type.")
		}
		name := strings.TrimSpace(row[cols["備註"]])
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		kind := "holiday"
		if off == "0" {
			if !weekend {
				continue
			}
			kind = "workday"
			if name == "" {
				name = "調整上班日"
			}
		} else {
			if name == "" {
				continue
			}
			if strings.Contains(name, "補假") || strings.Contains(name, "調整放假") {
				kind = "observed"
			}
		}
		records = append(records, HolidayRecord{Date: date, Name: name, Kind: kind, National: true})
	}
	expected := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC).Sub(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24
	if len(seen) != int(expected) {
		return nil, errors.New("Incomplete Taiwan calendar.")
	}
	return records, validateRecords(records, year)
}
func (s *holidayService) taiwanURL(ctx context.Context, year int) (string, error) {
	type resource struct {
		Description string `json:"resourceDescription"`
		Format      string `json:"resourceFormat"`
		URL         string `json:"resourceDownloadUrl"`
	}
	var catalog struct {
		Success bool
		Result  struct{ Distribution []resource }
	}
	validate := func(b []byte) error {
		if json.Unmarshal(b, &catalog) != nil || !catalog.Success || len(catalog.Result.Distribution) == 0 {
			return errors.New("Invalid Taiwan data catalog.")
		}
		return nil
	}
	b, _, err := s.resource(ctx, "tw-catalog", s.twURL, validate)
	if err != nil {
		return "", err
	}
	if err = validate(b); err != nil {
		return "", err
	}
	chosen := ""
	prefix := strconv.Itoa(year-1911) + "年"
	for _, item := range catalog.Result.Distribution {
		if !strings.HasPrefix(item.Description, prefix) || item.Format != "CSV" || strings.Contains(item.Description, "Google") {
			continue
		}
		u, err := url.Parse(item.URL)
		if err != nil || u.Scheme != "https" || u.Host != "www.dgpa.gov.tw" || u.Path != "/FileConversion" {
			continue
		}
		chosen = item.URL
	}
	if chosen == "" {
		return "", errors.New("This year's Taiwan calendar has not been published.")
	}
	return chosen, nil
}
func (s *holidayService) records(ctx context.Context, country string, year int) ([]HolidayRecord, bool, error) {
	key := fmt.Sprintf("%s-%d", country, year)
	parser := func(b []byte) ([]HolidayRecord, error) { return parseNager(b, country, year) }
	address := fmt.Sprintf("%s/Holidays/%s/%d", s.baseURL, country, year)
	if country == "CN" {
		address = fmt.Sprintf("%s/%d.json", s.cnURL, year)
		parser = func(b []byte) ([]HolidayRecord, error) { return parseChina(b, year) }
	}
	if country == "TW" {
		parser = func(b []byte) ([]HolidayRecord, error) { return parseTaiwan(b, year) }
		if cached, ok := s.state.Cache[key]; ok && time.Since(cached.Updated) < holidayRefresh {
			if records, err := parser(cached.Data); err == nil {
				return records, false, nil
			}
		}
		var err error
		address, err = s.taiwanURL(ctx, year)
		if err != nil {
			if cached, ok := s.state.Cache[key]; ok {
				if records, e := parser(cached.Data); e == nil {
					return records, true, nil
				}
			}
			return nil, false, err
		}
	}
	b, stale, err := s.resource(ctx, key, address, func(b []byte) error { _, err := parser(b); return err })
	if err != nil {
		return nil, stale, err
	}
	records, err := parser(b)
	return records, stale, err
}
func holidayYear(r *http.Request) (int, error) {
	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil || year < 1900 || year > 2200 {
		return 0, errors.New("Year must be between 1900 and 2200.")
	}
	return year, nil
}
func (s *APIServer) holidaySettings(w http.ResponseWriter, r *http.Request) {
	h := s.holidays
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loadError != nil {
		writeError(w, 500, "Saved holiday settings could not be read. The file has been preserved.")
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, h.state.Settings)
		return
	}
	defer r.Body.Close()
	var next HolidaySettings
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&next) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, 400, "Invalid holiday settings.")
		return
	}
	if err := validateHolidaySettings(next); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if next.Country != "" {
		countries, _, err := h.countries(r.Context())
		known := false
		for _, country := range countries {
			if country.Code == next.Country {
				known = true
			}
		}
		if !known {
			status := 400
			if err != nil {
				status = 503
			}
			writeError(w, status, "This country is unavailable. Reload the country list and try again.")
			return
		}
	}
	previous := h.state.Settings
	h.state.Settings = next
	if err := h.persist(); err != nil {
		h.state.Settings = previous
		writeError(w, 500, "Could not save holiday settings.")
		return
	}
	writeJSON(w, 200, next)
}
func (s *APIServer) holidayCountries(w http.ResponseWriter, r *http.Request) {
	h := s.holidays
	h.mu.Lock()
	defer h.mu.Unlock()
	items, stale, err := h.countries(r.Context())
	message := ""
	if err != nil {
		message = "Country list unavailable. Taiwan and China remain available."
	}
	writeJSON(w, 200, map[string]any{"countries": items, "stale": stale, "error": message})
}
func (s *APIServer) holidaySubdivisions(w http.ResponseWriter, r *http.Request) {
	country := r.URL.Query().Get("country")
	if !countryCodePattern.MatchString(country) {
		writeError(w, 400, "Invalid country.")
		return
	}
	year, err := holidayYear(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if country == "TW" || country == "CN" {
		writeJSON(w, 200, []holidaySubdivision{})
		return
	}
	h := s.holidays
	h.mu.Lock()
	defer h.mu.Unlock()
	records, _, err := h.records(r.Context(), country, year)
	if err != nil {
		writeError(w, 503, "Regional holiday data is unavailable. Try again later.")
		return
	}
	codes := map[string]bool{}
	for _, record := range records {
		for _, code := range record.Subdivisions {
			if strings.HasPrefix(code, country+"-") {
				codes[code] = true
			}
		}
	}
	items := []holidaySubdivision{}
	for code := range codes {
		name := subdivisionNames[strings.ToLower(strings.ReplaceAll(code, "-", ""))]
		if name == "" {
			name = code
		}
		items = append(items, holidaySubdivision{code, name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeJSON(w, 200, items)
}
func (s *APIServer) getHolidays(w http.ResponseWriter, r *http.Request) {
	year, err := holidayYear(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	h := s.holidays
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loadError != nil {
		writeError(w, 500, "Saved holiday settings could not be read.")
		return
	}
	selection := h.state.Settings
	records := []HolidayRecord{}
	stale := false
	available := true
	message := ""
	if selection.Country != "" {
		all, isStale, e := h.records(r.Context(), selection.Country, year)
		stale = isStale
		if e != nil {
			available = false
			message = "Holiday data unavailable for this year."
		} else {
			for _, record := range all {
				include := record.National
				for _, code := range record.Subdivisions {
					if code == selection.Subdivision {
						include = true
					}
				}
				if include {
					records = append(records, record)
				}
			}
		}
	}
	source := "https://nagerholidays.com"
	if selection.Country == "TW" {
		source = "https://data.gov.tw/dataset/14718"
	}
	if selection.Country == "CN" {
		source = "https://github.com/NateScarlet/holiday-cn"
	}
	if selection.Country == "" {
		source = ""
	}
	writeJSON(w, 200, map[string]any{"country": selection.Country, "subdivision": selection.Subdivision, "year": year, "available": available, "stale": stale, "source": source, "records": records, "error": message})
}

//go:embed holidays.html
var holidaysHTML string

func (s *APIServer) holidaysPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, holidaysHTML)
}
