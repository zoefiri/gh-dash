package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/branchsidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issueview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/sidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tabs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

const groupIconGlyph = ""

func newGroupingUiModel(t *testing.T) (*Model, *prssection.Model) {
	t.Helper()
	zone.NewGlobal()
	zone.SetEnabled(false)

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	cfg.JiraBaseUrl = "https://justpark.atlassian.net"

	ctx := &context.ProgramContext{
		Config:       &cfg,
		ScreenWidth:  160,
		ScreenHeight: 50,
		View:         config.PRsView,
		StartTask:    func(task context.Task) tea.Cmd { return nil },
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)

	prSection := prssection.NewModel(
		0,
		ctx,
		config.PrsSectionConfig{Title: "My Pull Requests", Filters: "is:open author:@me"},
		time.Now(),
		time.Now(),
	)

	m := &Model{
		ctx:              ctx,
		keys:             keys.Keys,
		prs:              []section.Section{&prSection},
		sidebar:          sidebar.NewModel(),
		footer:           footer.NewModel(ctx),
		tabs:             tabs.NewModel(ctx),
		prView:           prview.NewModel(ctx),
		issueSidebar:     issueview.NewModel(ctx),
		branchSidebar:    branchsidebar.NewModel(ctx),
		notificationView: notificationview.NewModel(ctx),
		tasks:            map[string]context.Task{},
	}
	m.syncMainContentDimensions()
	m.syncProgramContext()
	return m, &prSection
}

func uiTestPr(number int, title string) prrow.Data {
	return prrow.Data{Primary: &data.PullRequestData{
		Number:     number,
		Title:      title,
		Repository: data.Repository{NameWithOwner: "parkhub/repo"},
	}}
}

// fetchPage delivers a page of PRs the way the real fetch command does.
func fetchPage(m *Model, sec *prssection.Model, taskId string, total int, hasNext bool, prs ...prrow.Data) {
	m.tasks[taskId] = context.Task{Id: taskId}
	sec.LastFetch.TaskId = taskId
	m.Update(constants.TaskFinishedMsg{
		SectionId:   sec.Id,
		SectionType: prssection.SectionType,
		TaskId:      taskId,
		Msg: prssection.SectionPullRequestsFetchedMsg{
			Prs:        prs,
			TotalCount: total,
			PageInfo:   data.PageInfo{HasNextPage: hasNext, StartCursor: taskId},
			TaskId:     taskId,
		},
	})
}

func realisticPrs() []prrow.Data {
	titles := []string{
		"(PV-6277) Add Recipients card",
		"(CS-101) Dead-letter failed deliveries",
		"(PV-4474) Rework pricing page",
		"(PV-6277) Add seasonPassRecipients query",
		"Bump dependencies",
		"(PV-4474) Pricing follow-up",
		"(PV-6277) Add recipients listing endpoint",
		"(CS-101) Retry policy",
	}
	prs := make([]prrow.Data, 0, 20)
	for i := range 20 {
		prs = append(prs, uiTestPr(i+1, fmt.Sprintf("%s #%d", titles[i%len(titles)], i+1)))
	}
	return prs
}

func pressKey(m *Model, code rune) {
	m.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
}

func groupKeys(sec *prssection.Model) []string {
	var keys []string
	for _, g := range sec.Table.Groups() {
		keys = append(keys, g.Key)
	}
	return keys
}

func prNumberList(sec *prssection.Model) []int {
	numbers := make([]int, len(sec.Prs))
	for i, pr := range sec.Prs {
		numbers[i] = pr.GetNumber()
	}
	return numbers
}

func TestGroupByTicketKeyThroughUi(t *testing.T) {
	m, sec := newGroupingUiModel(t)
	fetchPage(m, sec, "t1", 27, true, realisticPrs()...)
	require.False(t, sec.Table.IsGrouped())
	require.NotContains(t, m.View().Content, groupIconGlyph)

	pressKey(m, 'b')

	require.Equal(t, []string{"PV-6277", "CS-101", "PV-4474", ""}, groupKeys(sec))
	view := m.View().Content
	require.Contains(t, view, groupIconGlyph)
	require.Contains(t, view, "justpark.atlassian.net/browse/PV-6277")
	require.Contains(t, view, "▾")

	pressKey(m, 'b')

	require.False(t, sec.Table.IsGrouped())
	require.NotContains(t, m.View().Content, groupIconGlyph)
	require.Equal(t, 1, prNumberList(sec)[0])
	require.Equal(t, 2, prNumberList(sec)[1], "fetched order restored")
}

func TestGroupingAppliesToLaterPagesAndRefreshes(t *testing.T) {
	m, sec := newGroupingUiModel(t)
	fetchPage(m, sec, "t1", 27, true, realisticPrs()...)
	pressKey(m, 'b')

	var more []prrow.Data
	for i := 21; i <= 27; i++ {
		more = append(more, uiTestPr(i, fmt.Sprintf("(PV-4474) more #%d", i)))
	}
	fetchPage(m, sec, "t2", 27, false, more...)
	require.Len(t, sec.Prs, 27)
	require.Equal(t, []string{"PV-6277", "CS-101", "PV-4474", ""}, groupKeys(sec))

	// manual refresh clears the rows, the refetched page is grouped again
	pressKey(m, 'r')
	require.Empty(t, sec.Prs)
	require.False(t, sec.Table.IsGrouped())
	fetchPage(m, sec, "t3", 27, true, realisticPrs()...)
	require.Equal(t, []string{"PV-6277", "CS-101", "PV-4474", ""}, groupKeys(sec))

	// background refetch restarts from the first page without clearing rows
	sec.PageInfo = nil
	fetchPage(m, sec, "t4", 27, true, realisticPrs()...)
	require.Equal(t, []string{"PV-6277", "CS-101", "PV-4474", ""}, groupKeys(sec))
	require.Contains(t, m.View().Content, groupIconGlyph)
}

func TestGroupByTicketRebindThroughUi(t *testing.T) {
	orig := keys.PRKeys.GroupByTicket
	defer func() { keys.PRKeys.GroupByTicket = orig }()
	keys.PRKeys.GroupByTicket.SetKeys("B")

	m, sec := newGroupingUiModel(t)
	fetchPage(m, sec, "t1", 27, true, realisticPrs()...)

	pressKey(m, 'b')
	require.False(t, sec.Table.IsGrouped(), "old key no longer groups")

	pressKey(m, 'B')
	require.True(t, sec.Table.IsGrouped())
}

func TestUserDefinedKeybindingTakesPrecedenceOverGroupByTicket(t *testing.T) {
	m, sec := newGroupingUiModel(t)
	m.ctx.Config.Keybindings.Prs = append(m.ctx.Config.Keybindings.Prs,
		config.Keybinding{Key: "b", Command: "echo hi"})
	fetchPage(m, sec, "t1", 27, true, realisticPrs()...)

	pressKey(m, 'b')

	require.False(t, sec.Table.IsGrouped(),
		"a custom command bound to b is handled before the built-in key")
}

func TestHelpListsGroupingKeys(t *testing.T) {
	m, sec := newGroupingUiModel(t)
	fetchPage(m, sec, "t1", 27, true, realisticPrs()...)

	pressKey(m, '?')

	view := m.View().Content
	require.Contains(t, view, "group by ticket")
	require.Contains(t, view, "collapse/expand group")
}

func TestToggleGroupKeyThroughUi(t *testing.T) {
	m, sec := newGroupingUiModel(t)
	fetchPage(m, sec, "t1", 27, true, realisticPrs()...)
	pressKey(m, 'b')
	require.Contains(t, m.View().Content, "Add Recipients card #1")

	pressKey(m, 'z')

	require.True(t, sec.Table.Groups()[0].Collapsed)
	view := m.View().Content
	require.Contains(t, view, groupIconGlyph)
	require.Contains(t, view, "▸")
	require.NotContains(t, view, "Add Recipients card #1")

	pressKey(m, 'z')

	require.False(t, sec.Table.Groups()[0].Collapsed)
	require.Contains(t, m.View().Content, "Add Recipients card #1")
}
