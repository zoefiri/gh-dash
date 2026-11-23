package prssection

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

const testJiraBaseUrl = "https://justpark.atlassian.net"

func TestExtractTicket(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"parenthesised key", "(PV-6277) Add Recipients card", "PV-6277"},
		{"key at start", "CS-101 Dead-letter messages", "CS-101"},
		{"key mid-title", "Fix the thing for PV-6279 properly", "PV-6279"},
		{"first of two keys", "PV-1 and PV-2 together", "PV-1"},
		{"alphanumeric project", "[AB2-17] tweak", "AB2-17"},
		{"no key", "Add recipients listing endpoint", ""},
		{"lowercase key", "pv-6277 lowercase", ""},
		{"no digits", "PV- missing number", ""},
		{"embedded in word", "fooPV-12 glued", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ExtractTicket(tt.title))
		})
	}
}

func TestTicketLink(t *testing.T) {
	link, label := ticketLink(testJiraBaseUrl, "PV-6279")
	require.Equal(t, "https://justpark.atlassian.net/browse/PV-6279", link)
	require.Equal(t, "justpark.atlassian.net/browse/PV-6279", label)

	link, label = ticketLink(testJiraBaseUrl+"/", "PV-6279")
	require.Equal(t, "https://justpark.atlassian.net/browse/PV-6279", link)
	require.Equal(t, "justpark.atlassian.net/browse/PV-6279", label)

	link, label = ticketLink("", "PV-6279")
	require.Empty(t, link)
	require.Empty(t, label)
}

func testPr(number int, title string) prrow.Data {
	return prrow.Data{Primary: &data.PullRequestData{
		Number:     number,
		Title:      title,
		Repository: data.Repository{NameWithOwner: "parkhub/repo"},
	}}
}

func newGroupingModel(t *testing.T) *Model {
	t.Helper()
	zone.NewGlobal()

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	cfg.JiraBaseUrl = testJiraBaseUrl

	th := theme.ParseTheme(&cfg)
	ctx := &context.ProgramContext{
		Config:            &cfg,
		Theme:             th,
		Styles:            context.InitStyles(th),
		MainContentWidth:  120,
		MainContentHeight: 40,
		StartTask:         func(task context.Task) tea.Cmd { return nil },
	}

	m := NewModel(0, ctx, config.PrsSectionConfig{Title: "test"}, time.Now(), time.Now())
	m.Table.SetDimensions(m.GetDimensions())
	return &m
}

func fetchPrs(m *Model, prs ...prrow.Data) {
	m.LastFetch.TaskId = "task"
	m.Update(SectionPullRequestsFetchedMsg{
		Prs:        prs,
		TotalCount: len(prs),
		PageInfo:   data.PageInfo{},
		TaskId:     "task",
	})
}

func press(m *Model, code rune) {
	m.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
}

func prNumbers(m *Model) []int {
	numbers := make([]int, len(m.Prs))
	for i, pr := range m.Prs {
		numbers[i] = pr.GetNumber()
	}
	return numbers
}

func currNumber(t *testing.T, m *Model) int {
	t.Helper()
	row := m.GetCurrRow()
	require.NotNil(t, row)
	return row.GetNumber()
}

func fetchedPrs() []prrow.Data {
	return []prrow.Data{
		testPr(1, "(PV-6277) Add recipients card"),
		testPr(2, "Bump dependencies"),
		testPr(3, "CS-101 Dead-letter failed deliveries"),
		testPr(4, "PV-6277 Add listing query"),
		testPr(5, "[PV-6279] Fix export"),
	}
}

func TestGroupByTicketOrdersByFirstAppearance(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	require.Equal(t, []int{1, 2, 3, 4, 5}, prNumbers(m))
	require.False(t, m.Table.IsGrouped())

	press(m, 'b')

	require.Equal(t, []int{1, 4, 3, 5, 2}, prNumbers(m))
	groups := m.Table.Groups()
	require.Len(t, groups, 4)

	require.Equal(t, "PV-6277", groups[0].Key)
	require.Equal(t, 0, groups[0].Start)
	require.Equal(t, 2, groups[0].End)
	require.Equal(t, "CS-101", groups[1].Key)
	require.Equal(t, "PV-6279", groups[2].Key)
	require.Equal(t, "https://justpark.atlassian.net/browse/PV-6279", groups[2].Link)
	require.Equal(t, "justpark.atlassian.net/browse/PV-6279", groups[2].LinkLabel)

	require.Equal(t, "", groups[3].Key)
	require.Equal(t, noTicketTitle, groups[3].Title)
	require.Empty(t, groups[3].Link)
	require.Equal(t, 4, groups[3].Start)
	require.Equal(t, 5, groups[3].End)
}

func TestUngroupRestoresFetchedOrder(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)

	press(m, 'b')
	require.Equal(t, []int{1, 4, 3, 5, 2}, prNumbers(m))

	press(m, 'b')
	require.Equal(t, []int{1, 2, 3, 4, 5}, prNumbers(m))
	require.False(t, m.Table.IsGrouped())
}

func TestGroupingKeepsCursorOnSamePr(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	m.Table.SetCurrItem(3)
	require.Equal(t, 4, currNumber(t, m))

	press(m, 'b')
	require.Equal(t, 4, currNumber(t, m))
	require.Equal(t, 1, m.Table.GetCurrItem())

	press(m, 'b')
	require.Equal(t, 4, currNumber(t, m))
	require.Equal(t, 3, m.Table.GetCurrItem())
}

func TestGroupingKeepsCursorOnSameHeader(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	press(m, 'b')
	m.Table.SelectHeader(1)
	require.Nil(t, m.GetCurrRow())

	fetchPrs(m, testPr(6, "PV-6277 another"))

	require.Equal(t, []int{1, 4, 6, 3, 5, 2}, prNumbers(m))
	require.Nil(t, m.GetCurrRow())
	gi := m.Table.CurrGroupIndex()
	require.Equal(t, "CS-101", m.Table.Groups()[gi].Key)
}

func TestFetchedPageIsGroupedWhenGrouped(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, testPr(1, "PV-1 one"), testPr(2, "PV-2 two"))
	press(m, 'b')

	m.PageInfo = nil
	fetchPrs(m, testPr(3, "PV-2 three"), testPr(4, "PV-1 four"), testPr(5, "PV-1 five"))

	require.Equal(t, []int{3, 4, 5}, prNumbers(m)[:3], "refetch restarts the fetched order")
	require.True(t, m.Table.IsGrouped())
}

func TestToggleCurrentGroupMovesCursorToHeader(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	press(m, 'b')
	m.Table.SetCurrItem(1)

	press(m, 'z')
	require.True(t, m.Table.Groups()[0].Collapsed)
	require.Nil(t, m.GetCurrRow(), "cursor moves to the collapsed group's header")
	require.Equal(t, 0, m.Table.CurrGroupIndex())
	require.NotContains(t, m.Table.View(), "Add listing query")

	press(m, 'z')
	require.False(t, m.Table.Groups()[0].Collapsed)
	require.Nil(t, m.GetCurrRow(), "cursor stays on the header")
	require.Contains(t, m.Table.View(), "Add listing query")

	m.Table.NextItem()
	require.Equal(t, 1, currNumber(t, m))
}

func TestToggleGroupIsNoOpWhenUngrouped(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)

	press(m, 'z')

	require.False(t, m.Table.IsGrouped())
	require.Equal(t, []int{1, 2, 3, 4, 5}, prNumbers(m))
}

func TestCollapsedStateSurvivesRegrouping(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	press(m, 'b')
	m.Table.SelectHeader(0)
	press(m, 'z')

	fetchPrs(m, testPr(6, "CS-101 more"))

	require.True(t, m.Table.Groups()[0].Collapsed)
	require.Equal(t, "PV-6277", m.Table.Groups()[0].Key)
}

func TestResetRowsClearsGroupingStateButKeepsMode(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	press(m, 'b')
	m.Table.SelectHeader(0)
	press(m, 'z')

	m.ResetRows()

	require.False(t, m.Table.IsGrouped())
	require.Empty(t, m.collapsed)
	require.Empty(t, m.order)
	require.True(t, m.grouped)

	fetchPrs(m, fetchedPrs()...)
	require.True(t, m.Table.IsGrouped())
	require.False(t, m.Table.Groups()[0].Collapsed)
}

func TestGroupingWithNoPrs(t *testing.T) {
	m := newGroupingModel(t)

	press(m, 'b')

	require.False(t, m.Table.IsGrouped())
	require.Nil(t, m.GetCurrRow())
}

func TestPagerShowsDashOnHeader(t *testing.T) {
	m := newGroupingModel(t)
	fetchPrs(m, fetchedPrs()...)
	press(m, 'b')
	m.Table.SelectHeader(0)

	require.Contains(t, m.GetPagerContent(), "PR -/5")

	m.Table.SetCurrItem(0)
	require.Contains(t, m.GetPagerContent(), "PR 1/5")
}

func TestHandleMouseClickToggleGroup(t *testing.T) {
	m := newGroupingModel(t)
	zone.SetEnabled(true)
	click := tea.MouseClickMsg(tea.Mouse{X: 3, Y: 0, Button: tea.MouseLeft})
	require.False(t, m.HandleMouseClick(click), "ignored when not grouped")

	fetchPrs(m, fetchedPrs()...)
	press(m, 'b')

	view := zone.Scan(m.Table.View())
	zoneId := m.Table.Groups()[0].ZoneID
	require.Eventually(t, func() bool {
		return !zone.Get(zoneId).IsZero()
	}, time.Second, 5*time.Millisecond)

	headerLine := -1
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "PV-6277") {
			headerLine = i
			break
		}
	}
	require.GreaterOrEqual(t, headerLine, 0, fmt.Sprintf("header not found in:\n%s", view))

	click = tea.MouseClickMsg(tea.Mouse{X: 3, Y: headerLine, Button: tea.MouseLeft})
	require.True(t, m.HandleMouseClick(click))
	require.True(t, m.Table.Groups()[0].Collapsed)

	outside := tea.MouseClickMsg(tea.Mouse{X: 3, Y: 1000, Button: tea.MouseLeft})
	require.False(t, m.HandleMouseClick(outside))
}
