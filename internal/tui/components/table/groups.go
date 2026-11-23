package table

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

const (
	groupIcon          = ""
	groupArrowExpanded = "▾"
	groupArrowCollapse = "▸"
)

// Group decorates the contiguous row range [Start, End) with a header row.
type Group struct {
	Key       string
	Title     string
	Link      string
	LinkLabel string
	Start     int
	End       int
	Collapsed bool
	ZoneID    string
}

// displayItem is either a group header or a table row.
type displayItem struct {
	header bool
	group  int
	row    int
}

func (m *Model) SetGroups(groups []Group) {
	wasGrouped := m.IsGrouped()
	m.groups = append([]Group(nil), groups...)
	if len(m.groups) == 0 {
		m.onHeader = false
		if wasGrouped {
			m.rowsViewport.AlignToCurrItem()
		}
		return
	}
	if m.onHeader && m.headerGroup >= len(m.groups) {
		m.onHeader = false
	}
}

func (m *Model) Groups() []Group {
	return m.groups
}

func (m *Model) IsGrouped() bool {
	return len(m.groups) > 0
}

// SelectHeader moves the cursor to the header of the given group.
func (m *Model) SelectHeader(group int) {
	if group < 0 || group >= len(m.groups) {
		return
	}
	m.onHeader = true
	m.headerGroup = group
}

// SetCurrItem moves the cursor to the given row.
func (m *Model) SetCurrItem(row int) {
	m.onHeader = false
	m.rowsViewport.SetCurrItem(row)
}

// CurrGroupIndex returns the group under the cursor, or -1 when there is none.
func (m *Model) CurrGroupIndex() int {
	if m.onHeader {
		return m.headerGroup
	}
	return m.groupOfRow(m.rowsViewport.GetCurrItem())
}

func (m *Model) groupOfRow(row int) int {
	for i, g := range m.groups {
		if row >= g.Start && row < g.End {
			return i
		}
	}
	return -1
}

// displayItems lists what is currently shown: headers, and the rows of groups
// that are not collapsed.
func (m *Model) displayItems() []displayItem {
	items := make([]displayItem, 0, len(m.Rows)+len(m.groups))
	next := 0
	for gi, g := range m.groups {
		for ; next < g.Start && next < len(m.Rows); next++ {
			items = append(items, displayItem{group: -1, row: next})
		}
		items = append(items, displayItem{header: true, group: gi, row: -1})
		end := min(g.End, len(m.Rows))
		if !g.Collapsed {
			for r := g.Start; r < end; r++ {
				items = append(items, displayItem{group: gi, row: r})
			}
		}
		next = max(next, end)
	}
	for ; next < len(m.Rows); next++ {
		items = append(items, displayItem{group: -1, row: next})
	}
	return items
}

// cursorItem returns the position of the cursor within items. A cursor on a
// row hidden by a collapsed group resolves to that group's header.
func (m *Model) cursorItem(items []displayItem) int {
	if m.onHeader {
		for i, it := range items {
			if it.header && it.group == m.headerGroup {
				return i
			}
		}
		return 0
	}

	row := m.rowsViewport.GetCurrItem()
	group := m.groupOfRow(row)
	for i, it := range items {
		if !it.header && it.row == row {
			return i
		}
		if it.header && it.group == group && m.groups[group].Collapsed {
			return i
		}
	}
	return 0
}

func (m *Model) selectItem(it displayItem) {
	if it.header {
		m.onHeader = true
		m.headerGroup = it.group
		return
	}
	m.onHeader = false
	m.rowsViewport.SetCurrItem(it.row)
}

func (m *Model) moveCursor(delta int) int {
	items := m.displayItems()
	if len(items) == 0 {
		return m.GetCurrItem()
	}
	idx := min(max(m.cursorItem(items)+delta, 0), len(items)-1)
	m.selectItem(items[idx])
	m.SyncViewPortContent()
	return m.GetCurrItem()
}

// moveCursorTo jumps to the first (0) or last (-1) displayed item.
func (m *Model) moveCursorTo(pos int) int {
	items := m.displayItems()
	if len(items) == 0 {
		return m.GetCurrItem()
	}
	if pos < 0 {
		pos = len(items) - 1
	}
	m.selectItem(items[min(pos, len(items)-1)])
	m.SyncViewPortContent()
	return m.GetCurrItem()
}

// syncGroupedContent renders headers and rows into the viewport and scrolls by
// line offset, since headers and rows differ in height.
func (m *Model) syncGroupedContent(headerColumns []string) {
	items := m.displayItems()
	cursor := m.cursorItem(items)

	rendered := make([]string, 0, len(items))
	offset, top, bottom := 0, 0, 0
	for i, it := range items {
		var s string
		if it.header {
			s = m.renderGroupHeader(it.group)
		} else {
			s = m.renderRow(it.row, headerColumns)
		}
		height := lipgloss.Height(s)
		if i == cursor {
			top, bottom = offset, offset+height
		}
		offset += height
		rendered = append(rendered, s)
	}

	m.rowsViewport.SyncViewPort(lipgloss.JoinVertical(lipgloss.Left, rendered...))
	m.rowsViewport.EnsureVisible(top, bottom)
}

func (m *Model) renderGroupHeader(groupId int) string {
	g := m.groups[groupId]
	theme := m.ctx.Theme

	base := lipgloss.NewStyle()
	if m.onHeader && m.headerGroup == groupId {
		base = base.Background(theme.SelectedBackground)
	}
	space := base.Render(" ")

	arrow := groupArrowExpanded
	if g.Collapsed {
		arrow = groupArrowCollapse
	}

	segments := []string{
		space,
		base.Foreground(theme.SecondaryText).Render(groupIcon),
		space,
		base.Bold(true).Foreground(theme.PrimaryText).Render(g.Title),
	}
	if g.LinkLabel != "" {
		label := g.LinkLabel
		if g.Link != "" {
			label = ansi.SetHyperlink(g.Link) + label + ansi.ResetHyperlink()
		}
		segments = append(segments, space, base.Foreground(theme.FaintText).Render(label))
	}
	segments = append(segments, space, base.Foreground(theme.PrimaryText).Render(arrow))

	line := strings.Join(segments, "")
	if gap := m.dimensions.Width - lipgloss.Width(line); gap > 0 {
		line += base.Render(strings.Repeat(" ", gap))
	}
	if g.ZoneID != "" {
		line = zone.Mark(g.ZoneID, line)
	}
	return line
}
