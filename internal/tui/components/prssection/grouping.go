package prssection

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
)

const noTicketTitle = "No ticket"

var ticketRegexp = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)

// ExtractTicket returns the first Jira-style ticket key found in title, or an
// empty string when there is none.
func ExtractTicket(title string) string {
	return ticketRegexp.FindString(title)
}

// ticketLink returns the link to a ticket and its short label (the link
// without the scheme). Both are empty when no base URL is configured.
func ticketLink(baseUrl, key string) (string, string) {
	baseUrl = strings.TrimRight(strings.TrimSpace(baseUrl), "/")
	if baseUrl == "" || key == "" {
		return "", ""
	}
	link := baseUrl + "/browse/" + key
	label := link
	if i := strings.Index(label, "://"); i >= 0 {
		label = label[i+len("://"):]
	}
	return link, label
}

func prOrderKey(pr prrow.Data) string {
	if pr.Primary == nil {
		return ""
	}
	return fmt.Sprintf("%s#%d", pr.GetRepoNameWithOwner(), pr.GetNumber())
}

func prTicket(pr prrow.Data) string {
	if pr.Primary == nil {
		return ""
	}
	return ExtractTicket(pr.GetTitle())
}

// recordOrder remembers the fetched position of every PR not seen before.
func (m *Model) recordOrder(prs []prrow.Data) {
	if m.order == nil {
		m.order = map[string]int{}
	}
	for _, pr := range prs {
		key := prOrderKey(pr)
		if _, ok := m.order[key]; !ok {
			m.order[key] = m.nextOrder
			m.nextOrder++
		}
	}
}

func (m *Model) resetOrder() {
	m.order = map[string]int{}
	m.nextOrder = 0
}

// sortPrs orders Prs by fetched order, or by ticket group first when grouped.
// Groups follow the first appearance of their ticket, and PRs without a ticket
// come last.
func (m *Model) sortPrs() {
	rank := map[string]int{}
	if m.grouped {
		for _, pr := range m.Prs {
			ticket := prTicket(pr)
			if ticket == "" {
				continue
			}
			pos := m.order[prOrderKey(pr)]
			if cur, ok := rank[ticket]; !ok || pos < cur {
				rank[ticket] = pos
			}
		}
	}

	groupRank := func(pr prrow.Data) int {
		if !m.grouped {
			return 0
		}
		if ticket := prTicket(pr); ticket != "" {
			return rank[ticket]
		}
		return math.MaxInt
	}

	slices.SortStableFunc(m.Prs, func(a, b prrow.Data) int {
		if c := cmp.Compare(groupRank(a), groupRank(b)); c != 0 {
			return c
		}
		return cmp.Compare(m.order[prOrderKey(a)], m.order[prOrderKey(b)])
	})
}

// buildGroups describes the contiguous ticket groups of the sorted Prs.
func (m *Model) buildGroups() []table.Group {
	if !m.grouped || len(m.Prs) == 0 {
		return nil
	}

	baseUrl := ""
	if m.Ctx != nil && m.Ctx.Config != nil {
		baseUrl = m.Ctx.Config.JiraBaseUrl
	}

	var groups []table.Group
	for i, pr := range m.Prs {
		ticket := prTicket(pr)
		if n := len(groups); n > 0 && groups[n-1].Key == ticket {
			groups[n-1].End = i + 1
			continue
		}

		g := table.Group{
			Key:       ticket,
			Title:     ticket,
			Start:     i,
			End:       i + 1,
			Collapsed: m.collapsed[ticket],
			ZoneID:    fmt.Sprintf("pr-group-%d-%s", m.Id, ticket),
		}
		if ticket == "" {
			g.Title = noTicketTitle
			g.ZoneID = fmt.Sprintf("pr-group-%d-none", m.Id)
		} else {
			g.Link, g.LinkLabel = ticketLink(baseUrl, ticket)
		}
		groups = append(groups, g)
	}
	return groups
}

// cursorAnchor identifies what the cursor is on: a PR (by order key) or a
// group header (by ticket key).
type cursorAnchor struct {
	prKey     string
	headerKey string
	onHeader  bool
}

func (m *Model) currentAnchor() cursorAnchor {
	if idx := m.Table.GetCurrItem(); idx >= 0 && idx < len(m.Prs) {
		return cursorAnchor{prKey: prOrderKey(m.Prs[idx])}
	}
	groups := m.Table.Groups()
	if gi := m.Table.CurrGroupIndex(); gi >= 0 && gi < len(groups) {
		return cursorAnchor{onHeader: true, headerKey: groups[gi].Key}
	}
	return cursorAnchor{}
}

func (m *Model) restoreAnchor(a cursorAnchor, groups []table.Group) {
	selectHeader := func(key string) bool {
		for i, g := range groups {
			if g.Key == key {
				m.Table.SelectHeader(i)
				return true
			}
		}
		return false
	}

	if a.onHeader {
		if !selectHeader(a.headerKey) {
			m.Table.SetCurrItem(0)
		}
		return
	}

	for i, pr := range m.Prs {
		if prOrderKey(pr) != a.prKey {
			continue
		}
		m.Table.SetCurrItem(i)
		if m.grouped && m.collapsed[prTicket(pr)] {
			selectHeader(prTicket(pr))
		}
		return
	}
}

// applyOrder reorders Prs for the current mode, rebuilds the groups and keeps
// the cursor on the same PR or header.
func (m *Model) applyOrder() {
	m.recordOrder(m.Prs)
	anchor := m.currentAnchor()

	m.sortPrs()
	groups := m.buildGroups()
	m.Table.SetGroups(groups)
	m.restoreAnchor(anchor, groups)
	m.Table.SetRows(m.BuildRows())
}

func (m *Model) toggleGrouping() {
	m.grouped = !m.grouped
	m.applyOrder()
}

func (m *Model) toggleCurrentGroup() {
	if !m.grouped {
		return
	}
	if gi := m.Table.CurrGroupIndex(); gi >= 0 {
		m.toggleGroupAt(gi)
	}
}

func (m *Model) toggleGroupAt(gi int) {
	groups := m.Table.Groups()
	if gi < 0 || gi >= len(groups) {
		return
	}
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}

	curGroup := m.Table.CurrGroupIndex()
	key := groups[gi].Key
	m.collapsed[key] = !m.collapsed[key]

	m.Table.SetGroups(m.buildGroups())
	if m.collapsed[key] && curGroup == gi {
		m.Table.SelectHeader(gi)
	}
	m.Table.SetRows(m.BuildRows())
}

// HandleMouseClick toggles the group whose header was clicked and reports
// whether the click was handled.
func (m *Model) HandleMouseClick(msg tea.MouseClickMsg) bool {
	if !m.grouped {
		return false
	}
	for i, g := range m.Table.Groups() {
		if g.ZoneID != "" && zone.Get(g.ZoneID).InBounds(msg) {
			m.toggleGroupAt(i)
			return true
		}
	}
	return false
}

// resetGrouping clears per-fetch grouping state. The grouped flag persists.
func (m *Model) resetGrouping() {
	m.collapsed = nil
	m.resetOrder()
	m.Table.SetGroups(nil)
}
