package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	cyanColor   = lipgloss.Color("#00F0FF")
	greenColor  = lipgloss.Color("#00E676")
	yellowColor = lipgloss.Color("#FFD600")
	redColor    = lipgloss.Color("#FF1744")
	grayColor   = lipgloss.Color("#666666")

	appBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyanColor).
			Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(cyanColor)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(cyanColor)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(grayColor)
)

type sessionState int

const (
	stateMenu sessionState = iota
	stateSnipeInput
	stateSniping
	stateRecents
	stateMonitorInput
	stateMonitoring
	stateAccounts
	stateAddAccountAlias
	stateAddAccountCookie
	stateSettings
	stateKeybinds
	stateRecordKey
)

type MenuItem struct {
	Name string
	Desc string
}

type tickMsg time.Time
type presencePollMsg struct {
	presences []UserPresence
	err       error
}
type resolveMsg struct {
	userID int64
	name   string
	err    error
}
type multiResolveMsg struct {
	userMap map[int64]string
	userIDs []int64
	err     error
}
type launchResultMsg struct {
	err error
}
type accountValidatedMsg struct {
	alias  string
	cookie string
	user   *AuthenticatedUserResponse
	err    error
}

type Model struct {
	width          int
	height         int
	state          sessionState
	cfg            *Config
	client         *RobloxClient
	menuCursor     int
	menuItems      []MenuItem
	textInput      textinput.Model
	spinner        spinner.Model
	statusText     string
	statusType     string
	targetUsername string
	targetID       int64
	targetFullName string
	isSniping      bool
	firstJobSeen   string
	launchMessage  string
	subCursor      int
	keybindCursor  int
	recordingKey   string
	monitorTargets map[int64]string
	monitorIDs     []int64
	monitorRows    []UserPresence
	tempAlias      string
}

func initialModel() Model {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = DefaultConfig()
	}

	ti := textinput.New()
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(cyanColor)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cyanColor)

	items := []MenuItem{
		{Name: "Snipe a player", Desc: "Wait for someone to join a game, then launch right in"},
		{Name: "Recent players", Desc: "Quickly pick from people you've sniped before"},
		{Name: "Watch players", Desc: "See what multiple friends or streamers are playing right now"},
		{Name: "Accounts", Desc: "Add or switch your Roblox accounts"},
		{Name: "Settings", Desc: "Change check speed, skip server, and keybinds"},
		{Name: "Exit", Desc: "Close RoSniper"},
	}

	return Model{
		width:     72,
		height:    24,
		state:     stateMenu,
		cfg:       cfg,
		client:    NewRobloxClient(),
		textInput: ti,
		spinner:   sp,
		menuItems: items,
	}
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) isUpKey(k string) bool {
	return k == m.cfg.Keybinds.Up || k == "w" || k == "W" || k == "up" || k == "k"
}

func (m Model) isDownKey(k string) bool {
	return k == m.cfg.Keybinds.Down || k == "s" || k == "S" || k == "down" || k == "j"
}

func (m Model) isConfirmKey(k string) bool {
	return k == m.cfg.Keybinds.Confirm || k == "v" || k == "V" || k == "enter"
}

func (m Model) isBackKey(k string) bool {
	return k == m.cfg.Keybinds.Back || k == "b" || k == "B" || k == "esc"
}

func resolveUserCmd(client *RobloxClient, username string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		id, name, err := client.ResolveUsername(ctx, username)
		return resolveMsg{userID: id, name: name, err: err}
	}
}

func pollPresenceCmd(client *RobloxClient, userIDs []int64, cookie string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		pres, err := client.GetPresence(ctx, userIDs, cookie)
		return presencePollMsg{presences: pres, err: err}
	}
}

func launchRobloxCmd(client *RobloxClient, placeID int64, gameID, cookie string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		var ticket string
		if cookie != "" {
			t, err := client.GetAuthTicket(ctx, cookie)
			if err == nil {
				ticket = t
			}
		}
		err := LaunchRoblox(placeID, gameID, ticket)
		return launchResultMsg{err: err}
	}
}

func multiResolveCmd(client *RobloxClient, users []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resMap := make(map[int64]string)
		var ids []int64
		for _, u := range users {
			id, name, err := client.ResolveUsername(ctx, u)
			if err == nil {
				resMap[id] = name
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return multiResolveMsg{err: fmt.Errorf("could not find any of those users")}
		}
		return multiResolveMsg{userMap: resMap, userIDs: ids, err: nil}
	}
}

func validateAccountCmd(client *RobloxClient, alias, cookie string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		user, err := client.ValidateCookie(ctx, cookie)
		return accountValidatedMsg{alias: alias, cookie: cookie, user: user, err: err}
	}
}

func pollDelayCmd(delay float64) tea.Cmd {
	return tea.Tick(time.Duration(delay*float64(time.Second)), func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		k := msg.String()

		if k == "ctrl+c" {
			return m, tea.Quit
		}

		if m.state == stateRecordKey {
			if k == "esc" {
				m.state = stateKeybinds
				return m, nil
			}
			switch m.recordingKey {
			case "Up":
				m.cfg.Keybinds.Up = strings.ToLower(k)
			case "Down":
				m.cfg.Keybinds.Down = strings.ToLower(k)
			case "Confirm":
				m.cfg.Keybinds.Confirm = strings.ToLower(k)
			case "Back":
				m.cfg.Keybinds.Back = strings.ToLower(k)
			}
			_ = m.cfg.Save()
			m.state = stateKeybinds
			return m, nil
		}

		// When not typing in a text field, handle global navigation
		isTyping := m.state == stateSnipeInput ||
			m.state == stateMonitorInput ||
			m.state == stateAddAccountAlias ||
			m.state == stateAddAccountCookie

		if !isTyping && m.isBackKey(k) && m.state != stateMenu {
			if m.state == stateKeybinds {
				m.state = stateSettings
				return m, nil
			}
			m.state = stateMenu
			m.isSniping = false
			m.statusText = ""
			m.textInput.Reset()
			return m, nil
		}

		if isTyping && k == "esc" {
			m.state = stateMenu
			m.textInput.Reset()
			return m, nil
		}

	case spinner.TickMsg:
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		return m, spinCmd

	case resolveMsg:
		if msg.err != nil {
			m.statusText = fmt.Sprintf("Could not find user: %v", msg.err)
			m.statusType = "error"
			m.isSniping = false
			return m, nil
		}
		m.targetID = msg.userID
		m.targetFullName = msg.name
		m.cfg.AddRecentTarget(m.targetUsername)
		m.statusText = "Looking for game..."
		m.statusType = "info"
		return m, pollPresenceCmd(m.client, []int64{m.targetID}, m.cfg.GetActiveCookie())

	case presencePollMsg:
		if !m.isSniping && m.state != stateMonitoring {
			return m, nil
		}

		if msg.err != nil {
			if strings.HasPrefix(msg.err.Error(), "RATE_LIMITED:") {
				parts := strings.Split(msg.err.Error(), ":")
				m.statusText = fmt.Sprintf("Checked too fast. Waiting %ss...", parts[1])
				m.statusType = "warn"
			} else {
				m.statusText = "Connection hiccup, retrying..."
				m.statusType = "warn"
			}
			return m, pollDelayCmd(m.cfg.RequestDelaySeconds)
		}

		if m.state == stateMonitoring {
			m.monitorRows = msg.presences
			return m, pollDelayCmd(m.cfg.RequestDelaySeconds)
		}

		if len(msg.presences) > 0 {
			p := msg.presences[0]
			switch p.UserPresenceType {
			case 0:
				m.statusText = "Offline"
				m.statusType = "error"
			case 1:
				m.statusText = "Online (Browsing website)"
				m.statusType = "info"
			case 3:
				m.statusText = "In Roblox Studio"
				m.statusType = "warn"
			case 2:
				if p.GameID == "" {
					m.statusText = fmt.Sprintf("In %s (Their joins are turned off)", p.LastLocation)
					m.statusType = "warn"
				} else {
					if m.cfg.DeclineFirstServer {
						if m.firstJobSeen == "" {
							m.firstJobSeen = p.GameID
							m.statusText = "Skipped initial server (waiting for switch)..."
							m.statusType = "warn"
							return m, pollDelayCmd(m.cfg.RequestDelaySeconds)
						} else if p.GameID == m.firstJobSeen {
							m.statusText = "Still in first server (waiting for switch)..."
							m.statusType = "info"
							return m, pollDelayCmd(m.cfg.RequestDelaySeconds)
						}
					}

					m.statusText = fmt.Sprintf("Found server in %s! Launching...", p.LastLocation)
					m.statusType = "success"
					return m, launchRobloxCmd(m.client, p.PlaceID, p.GameID, m.cfg.GetActiveCookie())
				}
			}
		}
		return m, pollDelayCmd(m.cfg.RequestDelaySeconds)

	case tickMsg:
		if m.isSniping {
			return m, pollPresenceCmd(m.client, []int64{m.targetID}, m.cfg.GetActiveCookie())
		}
		if m.state == stateMonitoring && len(m.monitorIDs) > 0 {
			return m, pollPresenceCmd(m.client, m.monitorIDs, m.cfg.GetActiveCookie())
		}

	case launchResultMsg:
		if msg.err != nil {
			m.launchMessage = fmt.Sprintf("Could not open Roblox: %v", msg.err)
		} else {
			m.launchMessage = "Roblox opened! Connecting to server..."
		}
		return m, nil

	case multiResolveMsg:
		if msg.err != nil {
			m.statusText = msg.err.Error()
			m.statusType = "error"
			return m, nil
		}
		m.monitorTargets = msg.userMap
		m.monitorIDs = msg.userIDs
		m.state = stateMonitoring
		return m, pollPresenceCmd(m.client, m.monitorIDs, m.cfg.GetActiveCookie())

	case accountValidatedMsg:
		if msg.err != nil {
			m.statusText = "Invalid cookie. Make sure you copied the whole thing."
			m.statusType = "error"
			m.state = stateAccounts
			return m, nil
		}
		m.cfg.Accounts = append(m.cfg.Accounts, Account{
			Alias:       msg.alias,
			Cookie:      msg.cookie,
			UserID:      msg.user.ID,
			Username:    msg.user.Name,
			DisplayName: msg.user.DisplayName,
			Valid:       true,
		})
		if m.cfg.ActiveAccount == "" {
			m.cfg.ActiveAccount = msg.alias
		}
		_ = m.cfg.Save()
		m.statusText = fmt.Sprintf("Added account @%s!", msg.user.Name)
		m.statusType = "success"
		m.state = stateAccounts
		return m, nil
	}

	// Sub-views
	switch m.state {
	case stateMenu:
		if msg, ok := msg.(tea.KeyMsg); ok {
			k := msg.String()
			if m.isUpKey(k) {
				if m.menuCursor > 0 {
					m.menuCursor--
				}
			} else if m.isDownKey(k) {
				if m.menuCursor < len(m.menuItems)-1 {
					m.menuCursor++
				}
			} else if m.isConfirmKey(k) {
				switch m.menuCursor {
				case 0:
					m.state = stateSnipeInput
					m.textInput.Placeholder = "Enter Roblox username"
					m.textInput.Focus()
				case 1:
					m.state = stateRecents
					m.subCursor = 0
				case 2:
					m.state = stateMonitorInput
					m.textInput.Placeholder = "User1, User2, User3"
					m.textInput.Focus()
				case 3:
					m.state = stateAccounts
					m.subCursor = 0
				case 4:
					m.state = stateSettings
					m.subCursor = 0
				case 5:
					return m, tea.Quit
				}
			}
		}

	case stateSnipeInput:
		m.textInput, cmd = m.textInput.Update(msg)
		if keyMsg, ok := msg.(tea.KeyMsg); ok && (keyMsg.String() == "enter" || m.isConfirmKey(keyMsg.String())) {
			target := strings.TrimSpace(m.textInput.Value())
			if target != "" {
				m.targetUsername = target
				m.state = stateSniping
				m.isSniping = true
				m.firstJobSeen = ""
				m.launchMessage = ""
				m.statusText = "Checking username..."
				m.statusType = "info"
				return m, resolveUserCmd(m.client, target)
			}
		}
		return m, cmd

	case stateRecents:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			k := keyMsg.String()
			if m.isUpKey(k) {
				if m.subCursor > 0 {
					m.subCursor--
				}
			} else if m.isDownKey(k) {
				if m.subCursor < len(m.cfg.RecentTargets)-1 {
					m.subCursor++
				}
			} else if m.isConfirmKey(k) {
				if len(m.cfg.RecentTargets) > 0 {
					m.targetUsername = m.cfg.RecentTargets[m.subCursor]
					m.state = stateSniping
					m.isSniping = true
					m.firstJobSeen = ""
					m.launchMessage = ""
					m.statusText = "Checking username..."
					m.statusType = "info"
					return m, resolveUserCmd(m.client, m.targetUsername)
				}
			}
		}

	case stateMonitorInput:
		m.textInput, cmd = m.textInput.Update(msg)
		if keyMsg, ok := msg.(tea.KeyMsg); ok && (keyMsg.String() == "enter" || m.isConfirmKey(keyMsg.String())) {
			raw := strings.Split(m.textInput.Value(), ",")
			var clean []string
			for _, u := range raw {
				if t := strings.TrimSpace(u); t != "" {
					clean = append(clean, t)
				}
			}
			if len(clean) > 0 {
				m.statusText = "Looking up players..."
				m.statusType = "info"
				return m, multiResolveCmd(m.client, clean)
			}
		}
		return m, cmd

	case stateAccounts:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			k := keyMsg.String()
			if m.isUpKey(k) {
				if m.subCursor > 0 {
					m.subCursor--
				}
			} else if m.isDownKey(k) {
				if m.subCursor < len(m.cfg.Accounts)-1 {
					m.subCursor++
				}
			} else if m.isConfirmKey(k) {
				if len(m.cfg.Accounts) > 0 {
					m.cfg.ActiveAccount = m.cfg.Accounts[m.subCursor].Alias
					_ = m.cfg.Save()
				}
			} else if k == "a" || k == "A" {
				m.state = stateAddAccountAlias
				m.textInput.Reset()
				m.textInput.Placeholder = "Nickname (e.g. Main, Alt)"
				m.textInput.Focus()
			} else if k == "d" || k == "D" {
				if len(m.cfg.Accounts) > 0 {
					m.cfg.Accounts = append(m.cfg.Accounts[:m.subCursor], m.cfg.Accounts[m.subCursor+1:]...)
					if m.subCursor >= len(m.cfg.Accounts) && m.subCursor > 0 {
						m.subCursor--
					}
					_ = m.cfg.Save()
				}
			}
		}

	case stateAddAccountAlias:
		m.textInput, cmd = m.textInput.Update(msg)
		if keyMsg, ok := msg.(tea.KeyMsg); ok && (keyMsg.String() == "enter" || m.isConfirmKey(keyMsg.String())) {
			m.tempAlias = strings.TrimSpace(m.textInput.Value())
			if m.tempAlias != "" {
				m.state = stateAddAccountCookie
				m.textInput.Reset()
				m.textInput.Placeholder = "Paste .ROBLOSECURITY cookie here"
				m.textInput.Focus()
			}
		}
		return m, cmd

	case stateAddAccountCookie:
		m.textInput, cmd = m.textInput.Update(msg)
		if keyMsg, ok := msg.(tea.KeyMsg); ok && (keyMsg.String() == "enter" || m.isConfirmKey(keyMsg.String())) {
			cookie := strings.TrimSpace(m.textInput.Value())
			if cookie != "" {
				m.statusText = "Checking cookie with Roblox..."
				m.statusType = "info"
				return m, validateAccountCmd(m.client, m.tempAlias, cookie)
			}
		}
		return m, cmd

	case stateSettings:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			k := keyMsg.String()
			if m.isUpKey(k) {
				if m.subCursor > 0 {
					m.subCursor--
				}
			} else if m.isDownKey(k) {
				if m.subCursor < 2 {
					m.subCursor++
				}
			} else if m.isConfirmKey(k) || k == " " {
				if m.subCursor == 0 {
					if m.cfg.RequestDelaySeconds == 1.0 {
						m.cfg.RequestDelaySeconds = 0.6
					} else if m.cfg.RequestDelaySeconds == 0.6 {
						m.cfg.RequestDelaySeconds = 2.0
					} else {
						m.cfg.RequestDelaySeconds = 1.0
					}
					_ = m.cfg.Save()
				} else if m.subCursor == 1 {
					m.cfg.DeclineFirstServer = !m.cfg.DeclineFirstServer
					_ = m.cfg.Save()
				} else if m.subCursor == 2 {
					m.state = stateKeybinds
					m.keybindCursor = 0
				}
			}
		}

	case stateKeybinds:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			k := keyMsg.String()
			if m.isUpKey(k) {
				if m.keybindCursor > 0 {
					m.keybindCursor--
				}
			} else if m.isDownKey(k) {
				if m.keybindCursor < 4 {
					m.keybindCursor++
				}
			} else if m.isConfirmKey(k) {
				switch m.keybindCursor {
				case 0:
					m.recordingKey = "Up"
					m.state = stateRecordKey
				case 1:
					m.recordingKey = "Down"
					m.state = stateRecordKey
				case 2:
					m.recordingKey = "Confirm"
					m.state = stateRecordKey
				case 3:
					m.recordingKey = "Back"
					m.state = stateRecordKey
				case 4:
					m.cfg.Keybinds = Keybinds{Up: "w", Down: "s", Confirm: "v", Back: "b"}
					_ = m.cfg.Save()
				}
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	boxWidth := 68
	if m.width > 20 && m.width < 74 {
		boxWidth = m.width - 4
	}

	activeAcc := "Not logged in"
	for _, a := range m.cfg.Accounts {
		if a.Alias == m.cfg.ActiveAccount {
			activeAcc = fmt.Sprintf("%s (@%s)", a.Alias, a.Username)
			break
		}
	}

	var speedName string
	switch m.cfg.RequestDelaySeconds {
	case 0.6:
		speedName = "Fast (0.6s)"
	case 2.0:
		speedName = "Slow (2.0s)"
	default:
		speedName = "Normal (1.0s)"
	}

	var skipServerText string
	if m.cfg.DeclineFirstServer {
		skipServerText = "On"
	} else {
		skipServerText = "Off"
	}

	header := titleStyle.Render("RoSniper") + "\n" +
		mutedStyle.Render(fmt.Sprintf("Account: %s  |  Speed: %s  |  Skip 1st: %s", activeAcc, speedName, skipServerText)) + "\n" +
		mutedStyle.Render(strings.Repeat("-", boxWidth-4)) + "\n\n"

	var body string
	switch m.state {
	case stateMenu:
		var items string
		for i, it := range m.menuItems {
			if m.menuCursor == i {
				items += selectedStyle.Render("> "+it.Name) + "\n"
				items += mutedStyle.Render("  "+it.Desc) + "\n\n"
			} else {
				items += normalStyle.Render("  "+it.Name) + "\n\n"
			}
		}
		body = items

	case stateSnipeInput:
		body = "Enter the Roblox username you want to snipe:\n\n" +
			m.textInput.View() + "\n\n" +
			mutedStyle.Render("[Enter/V: Start | Esc: Back]")

	case stateSniping:
		var statusColor lipgloss.Color
		switch m.statusType {
		case "success":
			statusColor = greenColor
		case "warn":
			statusColor = yellowColor
		case "error":
			statusColor = redColor
		default:
			statusColor = cyanColor
		}

		body = fmt.Sprintf("Target: %s\n\n", lipgloss.NewStyle().Bold(true).Render(m.targetUsername)) +
			fmt.Sprintf("Status: %s %s\n\n", m.spinner.View(), lipgloss.NewStyle().Foreground(statusColor).Render(m.statusText))

		if m.launchMessage != "" {
			body += lipgloss.NewStyle().Foreground(greenColor).Bold(true).Render(m.launchMessage) + "\n\n"
		}
		body += mutedStyle.Render(fmt.Sprintf("[%s / Esc: Stop sniping]", strings.ToUpper(m.cfg.Keybinds.Back)))

	case stateRecents:
		if len(m.cfg.RecentTargets) == 0 {
			body = "No recent targets yet.\n\n" + mutedStyle.Render(fmt.Sprintf("[%s / Esc: Back]", strings.ToUpper(m.cfg.Keybinds.Back)))
		} else {
			var list string
			for i, t := range m.cfg.RecentTargets {
				if m.subCursor == i {
					list += selectedStyle.Render(fmt.Sprintf("> %s\n", t))
				} else {
					list += normalStyle.Render(fmt.Sprintf("  %s\n", t))
				}
			}
			body = "Pick a player to snipe:\n\n" + list + "\n" +
				mutedStyle.Render(fmt.Sprintf("[%s / Enter: Snipe | %s / Esc: Back]", strings.ToUpper(m.cfg.Keybinds.Confirm), strings.ToUpper(m.cfg.Keybinds.Back)))
		}

	case stateMonitorInput:
		body = "Enter usernames separated by commas (e.g. KreekCraft, Flamingo):\n\n" +
			m.textInput.View() + "\n\n" +
			mutedStyle.Render("[Enter/V: Watch | Esc: Back]")

	case stateMonitoring:
		var rows string
		rows += fmt.Sprintf("%-20s %-16s %s\n", "Player", "Status", "Game")
		rows += strings.Repeat("-", boxWidth-6) + "\n"

		for _, p := range m.monitorRows {
			name := m.monitorTargets[p.UserID]
			var st string
			switch p.UserPresenceType {
			case 0:
				st = lipgloss.NewStyle().Foreground(redColor).Render("Offline")
			case 1:
				st = lipgloss.NewStyle().Foreground(cyanColor).Render("Online (Web)")
			case 2:
				st = lipgloss.NewStyle().Foreground(greenColor).Render("Playing")
			case 3:
				st = lipgloss.NewStyle().Foreground(yellowColor).Render("In Studio")
			}
			gameName := p.LastLocation
			if gameName == "" {
				gameName = "-"
			}
			rows += fmt.Sprintf("%-20s %-16s %s\n", name, st, gameName)
		}
		rows += fmt.Sprintf("\n%s Refreshing every %.1fs... %s", m.spinner.View(), m.cfg.RequestDelaySeconds, mutedStyle.Render(fmt.Sprintf("[%s / Esc: Stop]", strings.ToUpper(m.cfg.Keybinds.Back))))
		body = rows

	case stateAccounts:
		var list string
		if len(m.cfg.Accounts) == 0 {
			list = "No accounts added yet. Press 'A' to add one.\n"
		} else {
			for i, a := range m.cfg.Accounts {
				mark := " "
				if a.Alias == m.cfg.ActiveAccount {
					mark = "*"
				}
				line := fmt.Sprintf("[%s] %s (@%s)", mark, a.Alias, a.Username)
				if m.subCursor == i {
					list += selectedStyle.Render("> "+line) + "\n"
				} else {
					list += normalStyle.Render("  "+line) + "\n"
				}
			}
		}
		body = "Accounts:\n\n" + list + "\n" +
			mutedStyle.Render(fmt.Sprintf("[%s / Enter: Use account | A: Add | D: Delete | %s / Esc: Back]", strings.ToUpper(m.cfg.Keybinds.Confirm), strings.ToUpper(m.cfg.Keybinds.Back)))

	case stateAddAccountAlias:
		body = "Enter a nickname for this account (e.g. Main, Alt):\n\n" +
			m.textInput.View() + "\n\n" +
			mutedStyle.Render("[Enter/V: Next | Esc: Cancel]")

	case stateAddAccountCookie:
		body = "Paste your .ROBLOSECURITY cookie:\n\n" +
			m.textInput.View() + "\n\n" +
			mutedStyle.Render("[Enter/V: Save | Esc: Cancel]")

	case stateSettings:
		opt1 := fmt.Sprintf("Check speed: %s", speedName)
		opt2 := fmt.Sprintf("Skip first server: %s", skipServerText)
		opt3 := "Customize keybinds..."

		opts := []string{opt1, opt2, opt3}
		var optStr string
		for i, o := range opts {
			if m.subCursor == i {
				optStr += selectedStyle.Render("> "+o) + "\n\n"
			} else {
				optStr += normalStyle.Render("  "+o) + "\n\n"
			}
		}

		body = "Settings:\n\n" + optStr +
			mutedStyle.Render(fmt.Sprintf("[%s / Enter: Change/Open | %s / Esc: Back]", strings.ToUpper(m.cfg.Keybinds.Confirm), strings.ToUpper(m.cfg.Keybinds.Back)))

	case stateKeybinds:
		kbOpts := []string{
			fmt.Sprintf("Move Up:      [%s] (Up Arrow also works)", strings.ToUpper(m.cfg.Keybinds.Up)),
			fmt.Sprintf("Move Down:    [%s] (Down Arrow also works)", strings.ToUpper(m.cfg.Keybinds.Down)),
			fmt.Sprintf("Confirm:      [%s] (Enter also works)", strings.ToUpper(m.cfg.Keybinds.Confirm)),
			fmt.Sprintf("Deny / Back:  [%s] (Esc also works)", strings.ToUpper(m.cfg.Keybinds.Back)),
			"Reset to defaults (W/S, V, B)",
		}

		var kbStr string
		for i, o := range kbOpts {
			if m.keybindCursor == i {
				kbStr += selectedStyle.Render("> "+o) + "\n\n"
			} else {
				kbStr += normalStyle.Render("  "+o) + "\n\n"
			}
		}

		body = "Keybind Settings:\n\n" + kbStr +
			mutedStyle.Render(fmt.Sprintf("[%s / Enter: Rebind | %s / Esc: Back]", strings.ToUpper(m.cfg.Keybinds.Confirm), strings.ToUpper(m.cfg.Keybinds.Back)))

	case stateRecordKey:
		body = fmt.Sprintf("Rebinding [%s]:\n\n", m.recordingKey) +
			lipgloss.NewStyle().Foreground(cyanColor).Bold(true).Render("Press the new key on your keyboard now...") + "\n\n" +
			mutedStyle.Render("(Press Esc to cancel)")
	}

	content := header + body
	card := appBoxStyle.Width(boxWidth).Render(content)
	footer := mutedStyle.Render(fmt.Sprintf(" [%s/%s or Arrows] Move  |  [%s / Enter] Select  |  [%s / Esc] Back  |  [Ctrl+C] Quit",
		strings.ToUpper(m.cfg.Keybinds.Up), strings.ToUpper(m.cfg.Keybinds.Down), strings.ToUpper(m.cfg.Keybinds.Confirm), strings.ToUpper(m.cfg.Keybinds.Back)))

	return "\n" + card + "\n" + footer + "\n"
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
