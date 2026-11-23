package table

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

func newGroupedTable(t *testing.T, numRows, height int, groups []Group) Model {
	t.Helper()

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	th := theme.ParseTheme(&cfg)
	ctx := context.ProgramContext{
		Config: &cfg,
		Theme:  th,
		Styles: context.InitStyles(th),
	}

	rows := make([]Row, numRows)
	for i := range rows {
		rows[i] = Row{fmt.Sprintf("row-%d", i)}
	}

	m := NewModel(
		ctx,
		constants.Dimensions{Width: 60, Height: height},
		time.Now(),
		time.Now(),
		[]Column{{Title: "Title", Grow: utils.BoolPtr(true)}},
		rows,
		"PR",
		nil,
		"",
		false,
	)
	m.SetGroups(groups)
	m.SyncViewPortContent()
	return m
}

func twoGroups() []Group {
	return []Group{
		{Key: "PV-6279", Title: "PV-6279", Start: 0, End: 2},
		{Key: "CS-101", Title: "CS-101", Start: 2, End: 5},
	}
}

func TestGroupedNavigation(t *testing.T) {
	m := newGroupedTable(t, 5, 40, twoGroups())

	require.True(t, m.IsGrouped())
	require.Equal(t, -1, m.FirstItem(), "first item is the first header")

	// header, row 0, row 1, header, row 2, row 3, row 4
	want := []int{0, 1, -1, 2, 3, 4, 4}
	for _, w := range want {
		require.Equal(t, w, m.NextItem())
	}

	require.Equal(t, 4, m.LastItem())
	require.Equal(t, 3, m.PrevItem())
	require.Equal(t, 2, m.PrevItem())
	require.Equal(t, -1, m.PrevItem(), "second header")
	require.Equal(t, -1, m.GetCurrItem())
	require.Equal(t, 1, m.PrevItem())
}

func TestGroupedNavigationSkipsCollapsedRows(t *testing.T) {
	groups := twoGroups()
	groups[0].Collapsed = true
	m := newGroupedTable(t, 5, 40, groups)

	require.Len(t, m.displayItems(), 5, "2 headers and the 3 rows of the open group")

	require.Equal(t, -1, m.FirstItem())
	require.Equal(t, -1, m.NextItem(), "next header, rows 0-1 are hidden")
	require.Equal(t, 2, m.NextItem())
}

func TestCollapsedRowCursorResolvesToHeader(t *testing.T) {
	groups := twoGroups()
	groups[0].Collapsed = true
	m := newGroupedTable(t, 5, 40, groups)
	m.SetCurrItem(1)

	items := m.displayItems()
	idx := m.cursorItem(items)
	require.True(t, items[idx].header)
	require.Equal(t, 0, items[idx].group)
	require.Equal(t, 0, m.CurrGroupIndex())
}

func TestSetCurrItemClearsHeaderSelection(t *testing.T) {
	m := newGroupedTable(t, 5, 40, twoGroups())
	m.SelectHeader(1)
	require.Equal(t, -1, m.GetCurrItem())
	require.Equal(t, 1, m.CurrGroupIndex())

	m.SetCurrItem(3)
	require.Equal(t, 3, m.GetCurrItem())
	require.Equal(t, 1, m.CurrGroupIndex())
}

func TestSetGroupsClearsStaleHeaderSelection(t *testing.T) {
	m := newGroupedTable(t, 5, 40, twoGroups())
	m.SelectHeader(1)

	m.SetGroups(twoGroups()[:1])
	require.NotEqual(t, -1, m.GetCurrItem(), "selected group no longer exists")

	m.SelectHeader(0)
	m.SetGroups(nil)
	require.False(t, m.IsGrouped())
	require.NotEqual(t, -1, m.GetCurrItem())
}

func TestUngroupedNavigationUnchanged(t *testing.T) {
	m := newGroupedTable(t, 3, 40, nil)

	require.Equal(t, 1, m.NextItem())
	require.Equal(t, 2, m.NextItem())
	require.Equal(t, 2, m.NextItem())
	require.Equal(t, 1, m.PrevItem())
}

func TestGroupHeaderRendering(t *testing.T) {
	groups := twoGroups()
	groups[0].Link = "https://justpark.atlassian.net/browse/PV-6279"
	groups[0].LinkLabel = "justpark.atlassian.net/browse/PV-6279"
	groups[1].Collapsed = true
	m := newGroupedTable(t, 5, 40, groups)

	view := m.View()
	require.Contains(t, view, "PV-6279")
	require.Contains(t, view, "justpark.atlassian.net/browse/PV-6279")
	require.Contains(t, view, groupIcon)
	require.Contains(t, view, groupArrowExpanded)
	require.Contains(t, view, groupArrowCollapse)
	require.Contains(t, view, "row-1")
	require.NotContains(t, view, "row-3", "rows of a collapsed group are hidden")
}

func TestGroupedScrollingKeepsCursorVisible(t *testing.T) {
	groups := []Group{
		{Key: "PV-1", Title: "PV-1", Start: 0, End: 4},
		{Key: "PV-2", Title: "PV-2", Start: 4, End: 8},
	}
	m := newGroupedTable(t, 8, 8, groups)

	m.LastItem()
	view := m.View()
	require.Contains(t, view, "row-7")
	require.NotContains(t, view, "row-0")

	m.FirstItem()
	view = m.View()
	require.Contains(t, view, "PV-1")
	require.NotContains(t, view, "row-7")

	// header 1, rows 0-3, header 2, then row 4
	for range 6 {
		m.NextItem()
	}
	require.Equal(t, 4, m.GetCurrItem())
	require.Contains(t, m.View(), "row-4")
}
