package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/patrickmn/go-cache"
)

//go:embed *.env
var config embed.FS

const listHeight = 14

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2).MarginTop(1)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
	rightPanelStyle   = lipgloss.NewStyle().MarginTop(3)
)

type menuItem string

const (
	lastMatch       menuItem = "Last match"
	nextMatch       menuItem = "Next match"
	nextFiveMatches menuItem = "Next 5 matches"
	lastFiveMatches menuItem = "Last 5 matches"
)

func getMenus() []menuItem {
	return []menuItem{
		lastMatch,
		lastFiveMatches,
		nextMatch,
		nextFiveMatches,
	}
}

func (i menuItem) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(menuItem)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type model struct {
	list       list.Model
	choice     string
	quitting   bool
	httpClient *httpClient
	cache      *cache.Cache
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				m.choice = string(i)
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m model) View() string {
	ctx := context.Background()
	leftPanel := m.list.View()
	choice := m.choice

	var rightPanel string

switchCase:
	switch choice {
	case string(lastMatch):
		{
			res, err := m.getLastMatch(ctx)
			if err != nil {
				rightPanel = err.Error()
				break
			}

			if len(res.Response) == 0 {
				rightPanel = "Match not found."
				break
			}

			r := res.Response[0]
			t, err := time.Parse(time.RFC3339, r.Fixture.Date)
			if err != nil {
				rightPanel = err.Error()
				break
			}

			formatted := t.Format("2006 Jan 2 3:04 PM")
			if isManUtdHomeMatch(r) {
				rightPanel = "[Home]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s (%d) vs %s (%d)", r.Teams.Home.Name, r.Goals.Home, r.Teams.Away.Name, r.Goals.Away) + fmt.Sprintf("  [%s]\n", r.League.Name)
			} else {
				rightPanel = "[Away]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s (%d) vs %s (%d)", r.Teams.Home.Name, r.Goals.Home, r.Teams.Away.Name, r.Goals.Away) + fmt.Sprintf("  [%s]\n", r.League.Name)
			}
		}

	case string(lastFiveMatches):
		{
			res, err := m.getLastFiveMatches(ctx)
			if err != nil {
				rightPanel = err.Error()
				break
			}

			if len(res.Response) == 0 {
				rightPanel = "Matches not found."
				break
			}

			for _, r := range res.Response {
				t, err := time.Parse(time.RFC3339, r.Fixture.Date)
				if err != nil {
					rightPanel = err.Error()
					break switchCase
				}
				formatted := t.Format("2006 Jan 2 3:04 PM")
				if isManUtdHomeMatch(r) {
					rightPanel = rightPanel + "[Home]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s (%d) vs %s (%d)", r.Teams.Home.Name, r.Goals.Home, r.Teams.Away.Name, r.Goals.Away) + fmt.Sprintf("  [%s]\n", r.League.Name)
				} else {
					rightPanel = rightPanel + "[Away]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s (%d) vs %s (%d)", r.Teams.Home.Name, r.Goals.Home, r.Teams.Away.Name, r.Goals.Away) + fmt.Sprintf("  [%s]\n", r.League.Name)
				}
			}
		}

	case string(nextMatch):
		{
			res, err := m.getNextMatch(ctx)
			if err != nil {
				rightPanel = err.Error()
				break
			}

			if len(res.Response) == 0 {
				rightPanel = "Match found."
				break
			}

			r := res.Response[0]
			t, err := time.Parse(time.RFC3339, r.Fixture.Date)
			if err != nil {
				rightPanel = err.Error()
				break
			}

			formatted := t.Format("2006 Jan 2 3:04 PM")
			if isManUtdHomeMatch(r) {
				rightPanel = "[Home]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s vs %s", r.Teams.Home.Name, r.Teams.Away.Name) + fmt.Sprintf("  [%s]\n", r.League.Name)
			} else {
				rightPanel = "[Away]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s vs %s", r.Teams.Home.Name, r.Teams.Away.Name) + fmt.Sprintf("  [%s]\n", r.League.Name)
			}
		}

	case string(nextFiveMatches):
		{
			res, err := m.getNextFiveMatches(ctx)
			if err != nil {
				rightPanel = err.Error()
				break
			}

			if len(res.Response) == 0 {
				rightPanel = "Matches not found."
				break
			}

			for _, r := range res.Response {
				t, err := time.Parse(time.RFC3339, r.Fixture.Date)
				if err != nil {
					rightPanel = err.Error()
					break switchCase
				}

				formatted := t.Format("2006 Jan 2 3:04 PM")
				if isManUtdHomeMatch(r) {
					rightPanel = rightPanel + "[Home]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s vs %s", r.Teams.Home.Name, r.Teams.Away.Name) + fmt.Sprintf("  [%s]\n", r.League.Name)
				} else {
					rightPanel = rightPanel + "[Away]  " + fmt.Sprintf("%s  ->  ", formatted) + fmt.Sprintf("%s vs %s", r.Teams.Home.Name, r.Teams.Away.Name) + fmt.Sprintf("  [%s]\n", r.League.Name)
				}
			}
		}
	}

	if m.quitting {
		return quitTextStyle.Render("Glory Glory Man United.")
	}

	rightPanel = rightPanelStyle.Render(rightPanel)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func main() {
	env, err := readEnv(config)
	if err != nil {
		log.Panic(err)
	}

	rapidapiHost := env["RAPIDAPI_HOST"]
	if rapidapiHost == "" {
		log.Panic("missing env: RAPIDAPI_HOST")
	}

	rapidapiKey := env["RAPIDAPI_KEY"]
	if rapidapiKey == "" {
		log.Panic("missing env: RAPIDAPI_KEY")
	}

	httpClient := newHttpClient(rapidapiHost, rapidapiKey)

	menus := getMenus()
	items := make([]list.Item, len(menus))
	for i, item := range menus {
		items[i] = list.Item(item)
	}

	const defaultWidth = 20

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "Glory Glory Man United"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle

	m := model{list: l, httpClient: httpClient, cache: cache.New(10*time.Minute, 15*time.Minute)}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

func readEnv(config embed.FS) (map[string]string, error) {
	env := make(map[string]string)
	file, err := config.Open(".env")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		arr := strings.Split(scanner.Text(), "=")
		env[arr[0]] = arr[1]
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return env, nil
}

type httpClient struct {
	rapidapiHost string
	rapidapiKey  string
	client       *http.Client
}

func newHttpClient(rapidapiHost, rapidapiKey string) *httpClient {
	return &httpClient{
		rapidapiHost: rapidapiHost,
		rapidapiKey:  rapidapiKey,
		client: &http.Client{
			Timeout: time.Second * 5,
		},
	}
}

func (c *httpClient) do(req *http.Request) (data []byte, err error) {
	req.Header.Add("x-rapidapi-host", c.rapidapiHost)
	req.Header.Add("x-rapidapi-key", c.rapidapiKey)

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsuccessful request: %s", req.URL.RequestURI())
	}

	data, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

const (
	manchesterUnitedID = 33
	premierLeagueID    = 39
	rapidapiBaseUrl    = "https://api-football-v1.p.rapidapi.com/v3"
)

type fixture struct {
	ID       int    `json:"id"`
	Date     string `json:"date"`
	Venue    venue  `json:"venue"`
	TimeZone string `json:"timezone"`
}

type venue struct {
	Name string `json:"name"`
	City string `json:"city"`
}

type league struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type response struct {
	Fixture fixture `json:"fixture"`
	League  league  `json:"league"`
	Teams   struct {
		Home struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"home"`
		Away struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"away"`
	} `json:"teams"`
	Goals struct {
		Home int `json:"home"`
		Away int `json:"away"`
	} `json:"goals"`
}

type fixtureResponse struct {
	Response []response `json:"response"`
}

func (m model) getLastMatch(ctx context.Context) (fixtureResponse, error) {
	cacheKey := "lastMatch"
	if cachedData, found := m.cache.Get(cacheKey); found {
		return cachedData.(fixtureResponse), nil
	}

	params := url.Values{}
	params.Add("team", strconv.Itoa(manchesterUnitedID))
	params.Add("last", "1")
	params.Add("status", "FT")
	params.Add("timezone", "Australia/Perth")

	url := fmt.Sprintf("%s/fixtures?%s", rapidapiBaseUrl, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fixtureResponse{}, err
	}

	data, err := m.httpClient.do(req)
	if err != nil {
		return fixtureResponse{}, err
	}

	var res fixtureResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return fixtureResponse{}, err
	}

	m.cache.Set(cacheKey, res, cache.DefaultExpiration)
	return res, nil
}

func (m model) getLastFiveMatches(ctx context.Context) (fixtureResponse, error) {
	cacheKey := "lastFiveMatches"
	if cachedData, found := m.cache.Get(cacheKey); found {
		return cachedData.(fixtureResponse), nil
	}

	params := url.Values{}
	params.Add("team", strconv.Itoa(manchesterUnitedID))
	params.Add("last", "5")
	params.Add("status", "FT")
	params.Add("timezone", "Australia/Perth")

	url := fmt.Sprintf("%s/fixtures?%s", rapidapiBaseUrl, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fixtureResponse{}, err
	}

	data, err := m.httpClient.do(req)
	if err != nil {
		return fixtureResponse{}, err
	}

	var res fixtureResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return fixtureResponse{}, err
	}

	m.cache.Set(cacheKey, res, cache.DefaultExpiration)
	return res, nil
}

func isManUtdHomeMatch(r response) bool {
	return r.Teams.Home.ID == int(manchesterUnitedID)
}

func (m model) getNextMatch(ctx context.Context) (fixtureResponse, error) {
	cacheKey := "nextMatch"
	if cachedData, found := m.cache.Get(cacheKey); found {
		return cachedData.(fixtureResponse), nil
	}

	params := url.Values{}
	params.Add("team", strconv.Itoa(manchesterUnitedID))
	params.Add("next", "1")
	params.Add("timezone", "Australia/Perth")

	url := fmt.Sprintf("%s/fixtures?%s", rapidapiBaseUrl, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fixtureResponse{}, err
	}

	data, err := m.httpClient.do(req)
	if err != nil {
		return fixtureResponse{}, err
	}

	var res fixtureResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return fixtureResponse{}, err
	}

	m.cache.Set(cacheKey, res, cache.DefaultExpiration)
	return res, nil
}

func (m model) getNextFiveMatches(ctx context.Context) (fixtureResponse, error) {
	cacheKey := "nextFiveMatches"
	if cachedData, found := m.cache.Get(cacheKey); found {
		return cachedData.(fixtureResponse), nil
	}

	params := url.Values{}
	params.Add("team", strconv.Itoa(manchesterUnitedID))
	params.Add("next", "5")
	params.Add("timezone", "Australia/Perth")

	url := fmt.Sprintf("%s/fixtures?%s", rapidapiBaseUrl, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fixtureResponse{}, err
	}

	data, err := m.httpClient.do(req)
	if err != nil {
		return fixtureResponse{}, err
	}

	var res fixtureResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return fixtureResponse{}, err
	}

	m.cache.Set(cacheKey, res, cache.DefaultExpiration)
	return res, nil
}
