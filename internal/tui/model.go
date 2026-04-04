package tui

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nats-io/nats.go"
)

// ─── View IDs ─────────────────────────────────────────────────────────────────

type ViewID int

const (
	PnLView ViewID = iota
	ActivityView
	OrderbooksView
	GuideView
)

// ─── Tea messages ─────────────────────────────────────────────────────────────

type tradeMsg models.Trade
type snapshotMsg models.OrderbookSnapshot
type priceMsg models.PriceTick

// ─── Styles ───────────────────────────────────────────────────────────────────

var (
	clrGreen    = lipgloss.Color("#00FF7F")
	clrRed      = lipgloss.Color("#FF5555")
	clrYellow   = lipgloss.Color("#FFD700")
	clrCyan     = lipgloss.Color("#00FFFF")
	clrGray     = lipgloss.Color("#888888")
	clrDimGray  = lipgloss.Color("#444444")
	clrActiveTab = lipgloss.Color("#00BFFF")

	sPositive = lipgloss.NewStyle().Foreground(clrGreen).Bold(true)
	sNegative = lipgloss.NewStyle().Foreground(clrRed).Bold(true)
	sGray     = lipgloss.NewStyle().Foreground(clrGray)
	sDim      = lipgloss.NewStyle().Foreground(clrDimGray)
	sYellow   = lipgloss.NewStyle().Foreground(clrYellow).Bold(true)
	sCyan     = lipgloss.NewStyle().Foreground(clrCyan)
	sAsk      = lipgloss.NewStyle().Foreground(clrRed)
	sBid      = lipgloss.NewStyle().Foreground(clrGreen)
)

// CRYPTOSIM in ANSI Shadow figlet font
var logoLines = []string{
	` ██████╗██████╗ ██╗   ██╗██████╗ ████████╗ ██████╗ ███████╗██╗███╗   ███╗`,
	`██╔════╝██╔══██╗╚██╗ ██╔╝██╔══██╗╚══██╔══╝██╔═══██╗██╔════╝██║████╗ ████║`,
	`██║     ██████╔╝ ╚████╔╝ ██████╔╝   ██║   ██║   ██║███████╗██║██╔████╔██║`,
	`██║     ██╔══██╗  ╚██╔╝  ██╔═══╝    ██║   ██║   ██║╚════██║██║██║╚██╔╝██║`,
	`╚██████╗██║  ██║   ██║   ██║        ██║   ╚██████╔╝███████║██║██║ ╚═╝ ██║`,
	` ╚═════╝╚═╝  ╚═╝   ╚═╝   ╚═╝        ╚═╝    ╚═════╝ ╚══════╝╚═╝╚═╝     ╚═╝`,
}

// Gradient: cyan → blue across logo rows
var logoColors = []lipgloss.Color{
	"#00FFFF", "#00E0FF", "#00C2FF", "#00A4FF", "#0085FF", "#0067FF",
}

// ─── Model ────────────────────────────────────────────────────────────────────

const maxActivity = 500

type Model struct {
	nc             *nats.Conn
	natsURL        string
	view           ViewID
	participants   map[string]*ParticipantState
	sortedIDs      []string
	trades         []models.Trade
	snapshots      map[string]models.OrderbookSnapshot
	tradeCh        chan models.Trade
	snapshotCh     chan models.OrderbookSnapshot
	priceCh        chan models.PriceTick
	width          int
	height         int
	activityOffset int
	totalTrades    int
}

func NewModel(nc *nats.Conn, natsURL string) *Model {
	m := &Model{
		nc:           nc,
		natsURL:      natsURL,
		view:         PnLView,
		participants: make(map[string]*ParticipantState),
		snapshots:    make(map[string]models.OrderbookSnapshot),
		tradeCh:      make(chan models.Trade, 2000),
		snapshotCh:   make(chan models.OrderbookSnapshot, 200),
		priceCh:      make(chan models.PriceTick, 200),
	}

	nc.Subscribe(models.TradesExecutedTopic, func(msg *nats.Msg) {
		var t models.Trade
		if json.Unmarshal(msg.Data, &t) == nil {
			select {
			case m.tradeCh <- t:
			default:
			}
		}
	})

	nc.Subscribe(models.OrderBookSnapshotTopic, func(msg *nats.Msg) {
		var s models.OrderbookSnapshot
		if json.Unmarshal(msg.Data, &s) == nil {
			select {
			case m.snapshotCh <- s:
			default:
			}
		}
	})

	for _, topic := range []string{models.PriceBTCTopic, models.PriceETHTopic, models.PriceXRPTopic} {
		t := topic
		nc.Subscribe(t, func(msg *nats.Msg) {
			var tick models.PriceTick
			if json.Unmarshal(msg.Data, &tick) == nil {
				select {
				case m.priceCh <- tick:
				default:
				}
			}
		})
	}

	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		awaitTrade(m.tradeCh),
		awaitSnapshot(m.snapshotCh),
		awaitPrice(m.priceCh),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.view = PnLView
		case "2":
			m.view = ActivityView
			m.activityOffset = 0
		case "3":
			m.view = OrderbooksView
		case "4", "?":
			m.view = GuideView
		case "up", "k":
			if m.activityOffset > 0 {
				m.activityOffset--
			}
		case "down", "j":
			if m.view == ActivityView {
				m.activityOffset++
			}
		}

	case tradeMsg:
		m.processTrade(models.Trade(msg))
		return m, awaitTrade(m.tradeCh)

	case snapshotMsg:
		s := models.OrderbookSnapshot(msg)
		m.snapshots[s.Symbol] = s
		return m, awaitSnapshot(m.snapshotCh)

	case priceMsg:
		m.updatePrice(models.PriceTick(msg))
		return m, awaitPrice(m.priceCh)
	}

	return m, nil
}

func isLoadTester(id string) bool {
	return strings.HasPrefix(id, "load-tester")
}

func (m *Model) processTrade(trade models.Trade) {
	if isLoadTester(trade.BuyerID) || isLoadTester(trade.SellerID) {
		return
	}

	m.totalTrades++
	m.trades = append(m.trades, trade)
	if len(m.trades) > maxActivity {
		m.trades = m.trades[1:]
	}

	for _, id := range []string{trade.BuyerID, trade.SellerID} {
		if id == "" {
			continue
		}
		if _, ok := m.participants[id]; !ok {
			m.participants[id] = NewParticipantState(id, trade.Symbol)
			m.rebuildSortedIDs()
		}
		m.participants[id].OnTrade(trade)
	}
}

func (m *Model) updatePrice(tick models.PriceTick) {
	for _, p := range m.participants {
		if p.Symbol == tick.Symbol {
			p.UpdateMid(tick.Mid)
		}
	}
}

func (m *Model) rebuildSortedIDs() {
	m.sortedIDs = make([]string, 0, len(m.participants))
	for id := range m.participants {
		m.sortedIDs = append(m.sortedIDs, id)
	}
	sort.Strings(m.sortedIDs)
}

// ─── Commands ────────────────────────────────────────────────────────────────

func awaitTrade(ch <-chan models.Trade) tea.Cmd {
	return func() tea.Msg { return tradeMsg(<-ch) }
}

func awaitSnapshot(ch <-chan models.OrderbookSnapshot) tea.Cmd {
	return func() tea.Msg { return snapshotMsg(<-ch) }
}

func awaitPrice(ch <-chan models.PriceTick) tea.Cmd {
	return func() tea.Msg { return priceMsg(<-ch) }
}

// ─── Top-level view ───────────────────────────────────────────────────────────

func (m *Model) View() string {
	var b strings.Builder

	b.WriteString(renderLogo())
	b.WriteString("\n")
	b.WriteString(renderNav(m.view))
	b.WriteString("\n\n")

	switch m.view {
	case PnLView:
		b.WriteString(m.renderPnL())
	case ActivityView:
		b.WriteString(m.renderActivity())
	case OrderbooksView:
		b.WriteString(m.renderOrderbooks())
	case GuideView:
		b.WriteString(renderGuide())
	}

	b.WriteString("\n\n")
	b.WriteString(m.renderStatus())
	b.WriteString("\n")
	b.WriteString(renderHelp())

	return b.String()
}

// ─── Header ───────────────────────────────────────────────────────────────────

func renderLogo() string {
	var lines []string
	for i, line := range logoLines {
		c := logoColors[i%len(logoColors)]
		lines = append(lines, lipgloss.NewStyle().Foreground(c).Bold(true).Render(line))
	}
	sub := sGray.Render("  real-time crypto exchange simulation  ·  NATS + Go + TimescaleDB")
	lines = append(lines, sub)
	return strings.Join(lines, "\n")
}

func renderNav(active ViewID) string {
	type tab struct {
		id    ViewID
		label string
	}
	tabs := []tab{
		{PnLView, "1  PnL"},
		{ActivityView, "2  Activity"},
		{OrderbooksView, "3  Order Books"},
		{GuideView, "4  Guide"},
	}

	var parts []string
	for _, t := range tabs {
		if t.id == active {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(clrActiveTab).
				Bold(true).
				Underline(true).
				Render("[ "+t.label+" ]"))
		} else {
			parts = append(parts, sGray.Render("  "+t.label+"  "))
		}
	}
	return strings.Join(parts, "  ")
}

func (m *Model) renderStatus() string {
	connected := "●"
	status := lipgloss.NewStyle().Foreground(clrGreen).Render(connected+" connected") +
		sGray.Render(" · "+m.natsURL) +
		sGray.Render(fmt.Sprintf(" · %d total trades · %d participants",
			m.totalTrades, len(m.participants)))
	return "  " + status
}

func renderHelp() string {
	keys := []string{
		sCyan.Render("1-4") + sGray.Render(" switch view"),
		sCyan.Render("j/k ↑↓") + sGray.Render(" scroll activity"),
		sCyan.Render("q") + sGray.Render(" quit"),
	}
	return "  " + strings.Join(keys, "   ")
}

// ─── PnL View ─────────────────────────────────────────────────────────────────

func (m *Model) renderPnL() string {
	if len(m.sortedIDs) == 0 {
		return sGray.Render("  waiting for trades — is the simulation running?\n")
	}

	sparkWidth := 60
	if m.width > 140 {
		sparkWidth = 80
	}

	var rows []string
	for _, id := range m.sortedIDs {
		snap := m.participants[id].Snapshot()

		pnlStr := formatPnL(snap.PnL)
		posLabel := fmt.Sprintf("pos %+.4f %-7s", snap.Position, snap.Symbol[:3])
		cashLabel := fmt.Sprintf("cash %+10.2f", snap.CashFlow)
		midLabel := fmt.Sprintf("mid %-12s", formatPrice(snap.Symbol, snap.MidPrice))
		tradeLabel := fmt.Sprintf("%d trades", snap.TradeCount)

		line1 := fmt.Sprintf("  %s  %s  %s  %s  %s  %s",
			sYellow.Render(fmt.Sprintf("%-22s", id)),
			sCyan.Render(snap.Symbol),
			pnlStr,
			sGray.Render(posLabel),
			sGray.Render(cashLabel),
			sDim.Render(tradeLabel),
		)

		spark := sparkline(snap.PnLHistory, sparkWidth)
		var sparkStyled string
		if snap.PnL >= 0 {
			sparkStyled = sPositive.Render(spark)
		} else {
			sparkStyled = sNegative.Render(spark)
		}
		midStr := sGray.Render(fmt.Sprintf("  %s", midLabel))
		line2 := "  " + sparkStyled + midStr

		rows = append(rows, line1+"\n"+line2)
	}

	return strings.Join(rows, "\n\n")
}

// ─── Activity View ────────────────────────────────────────────────────────────

func (m *Model) renderActivity() string {
	if len(m.trades) == 0 {
		return sGray.Render("  no trades yet — waiting for activity...\n")
	}

	header := fmt.Sprintf("  %-14s %-9s %-22s %-22s %-10s %s",
		"TIME", "SYMBOL", "BUYER", "SELLER", "QTY", "PRICE")
	headerStyled := sDim.Render(header)

	// Show newest first
	all := m.trades
	visibleLines := m.height - 18 // rough estimate of non-content lines
	if visibleLines < 5 {
		visibleLines = 5
	}

	start := len(all) - 1 - m.activityOffset
	if start < 0 {
		start = 0
	}
	end := start - visibleLines
	if end < 0 {
		end = -1
	}

	var rows []string
	rows = append(rows, headerStyled)
	rows = append(rows, sDim.Render("  "+strings.Repeat("─", 85)))

	for i := start; i > end; i-- {
		t := all[i]
		ts := t.ExecutedAt.Format("15:04:05.000")
		qty := fmt.Sprintf("%.6g", t.Qty)
		price := formatPrice(t.Symbol, t.Price)

		var buyerStr, sellerStr string
		if t.BuyerID == "" {
			buyerStr = sGray.Render("—")
		} else {
			buyerStr = sBid.Render(t.BuyerID)
		}
		if t.SellerID == "" {
			sellerStr = sGray.Render("—")
		} else {
			sellerStr = sAsk.Render(t.SellerID)
		}

		row := fmt.Sprintf("  %s  %s  %-22s  %-22s  %-10s  %s",
			sGray.Render(ts),
			sCyan.Render(t.Symbol),
			buyerStr,
			sellerStr,
			sGray.Render(qty),
			sYellow.Render(price),
		)
		rows = append(rows, row)
	}

	scrollHint := ""
	if m.activityOffset > 0 {
		scrollHint = sGray.Render(fmt.Sprintf("  ↑ scrolled back %d  (j/k to navigate)", m.activityOffset))
	}

	result := strings.Join(rows, "\n")
	if scrollHint != "" {
		result += "\n" + scrollHint
	}
	return result
}

// ─── Orderbooks View ─────────────────────────────────────────────────────────

const obDepth = 15 // ask/bid levels to show per side

func (m *Model) renderOrderbooks() string {
	symbols := []string{
		string(models.BTC_USD),
		string(models.ETH_USD),
		string(models.XRP_USD),
	}

	var cols []string
	for _, sym := range symbols {
		cols = append(cols, m.renderBook(sym))
	}

	// Join columns side by side
	colLines := make([][]string, len(cols))
	maxH := 0
	for i, col := range cols {
		colLines[i] = strings.Split(col, "\n")
		if len(colLines[i]) > maxH {
			maxH = len(colLines[i])
		}
	}

	// Pad all columns to same height
	colW := 30
	if m.width > 100 {
		colW = (m.width - 6) / 3
	}
	for i := range colLines {
		for len(colLines[i]) < maxH {
			colLines[i] = append(colLines[i], strings.Repeat(" ", colW))
		}
	}

	var rows []string
	for row := 0; row < maxH; row++ {
		var parts []string
		for _, col := range colLines {
			parts = append(parts, col[row])
		}
		rows = append(rows, strings.Join(parts, "  "))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) renderBook(symbol string) string {
	snap, ok := m.snapshots[symbol]

	colW := 28

	title := lipgloss.NewStyle().
		Foreground(clrYellow).
		Bold(true).
		Width(colW).
		Render("  " + symbol)

	divider := sDim.Render(strings.Repeat("─", colW))
	headerLine := sDim.Render(fmt.Sprintf("  %-14s  %10s", "PRICE", "QTY"))

	var lines []string
	lines = append(lines, title)
	lines = append(lines, divider)
	lines = append(lines, headerLine)

	if !ok || (len(snap.Asks) == 0 && len(snap.Bids) == 0) {
		lines = append(lines, sGray.Render("  waiting for data..."))
		return strings.Join(lines, "\n")
	}

	// Show asks in reverse order (highest ask on top, lowest near spread)
	asks := snap.Asks
	if len(asks) > obDepth {
		asks = asks[:obDepth]
	}
	for i := len(asks) - 1; i >= 0; i-- {
		a := asks[i]
		lines = append(lines, sAsk.Render(
			fmt.Sprintf("  %-14s  %10.6g", formatPrice(symbol, a[0]), a[1]),
		))
	}

	// Spread
	var spreadStr string
	if len(snap.Asks) > 0 && len(snap.Bids) > 0 {
		spread := snap.Asks[0][0] - snap.Bids[0][0]
		spreadPct := (spread / snap.Bids[0][0]) * 100
		spreadStr = fmt.Sprintf("── spread: %s (%.4f%%) ──",
			formatPrice(symbol, spread), spreadPct)
	} else {
		spreadStr = "── spread: n/a ──"
	}
	lines = append(lines, lipgloss.NewStyle().
		Foreground(clrActiveTab).
		Width(colW).
		Render(spreadStr))

	// Bids (best bid first)
	bids := snap.Bids
	if len(bids) > obDepth {
		bids = bids[:obDepth]
	}
	for _, b := range bids {
		lines = append(lines, sBid.Render(
			fmt.Sprintf("  %-14s  %10.6g", formatPrice(symbol, b[0]), b[1]),
		))
	}

	return strings.Join(lines, "\n")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

var sparkBlocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func sparkline(values []float64, width int) string {
	if len(values) == 0 {
		return strings.Repeat(" ", width)
	}

	data := values
	if len(data) > width {
		data = data[len(data)-width:]
	}

	minV, maxV := data[0], data[0]
	for _, v := range data {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	var sb strings.Builder
	for _, v := range data {
		var idx int
		if maxV > minV {
			idx = int(math.Round((v - minV) / (maxV - minV) * float64(len(sparkBlocks)-1)))
		} else {
			idx = 4
		}
		idx = max(0, min(idx, len(sparkBlocks)-1))
		sb.WriteRune(sparkBlocks[idx])
	}

	for sb.Len() < width {
		sb.WriteByte(' ')
	}
	return sb.String()
}

func formatPnL(v float64) string {
	if v >= 0 {
		return sPositive.Render(fmt.Sprintf("+$%12.2f", v))
	}
	return sNegative.Render(fmt.Sprintf("-$%12.2f", math.Abs(v)))
}

func formatPrice(symbol string, price float64) string {
	switch symbol {
	case string(models.BTC_USD):
		return fmt.Sprintf("$%.2f", price)
	case string(models.ETH_USD):
		return fmt.Sprintf("$%.2f", price)
	case string(models.XRP_USD):
		return fmt.Sprintf("$%.4f", price)
	default:
		return fmt.Sprintf("%.6g", price)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderGuide returns the feature guide for the help view (shown on startup).
func renderGuide() string {
	style := lipgloss.NewStyle().Foreground(clrGray)
	bold := lipgloss.NewStyle().Foreground(clrCyan).Bold(true)
	dim := lipgloss.NewStyle().Foreground(clrDimGray)

	sections := []struct{ key, val string }{
		{"1  PnL Dashboard", "per-participant mark-to-market PnL, position, cash flow, and sparkline graph"},
		{"2  Activity Feed", "live trade stream with buyer/seller IDs, qty, price — scroll with j/k"},
		{"3  Order Books", "real-time top-30 bid/ask ladder for BTC-USD, ETH-USD, XRP-USD"},
	}

	var lines []string
	lines = append(lines, bold.Render("  Features"))
	lines = append(lines, dim.Render("  "+strings.Repeat("─", 70)))
	for _, s := range sections {
		lines = append(lines, fmt.Sprintf("  %s   %s",
			bold.Render(fmt.Sprintf("%-20s", s.key)),
			style.Render(s.val),
		))
	}
	lines = append(lines, "")
	lines = append(lines, dim.Render("  PnL is mark-to-market: cash flow + open position × current mid price."))
	lines = append(lines, dim.Render("  Sparklines scale to the participant's own PnL range."))
	return strings.Join(lines, "\n")
}
