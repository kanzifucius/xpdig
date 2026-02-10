package xpnavigator

import (
	"fmt"
	"time"

	"github.com/brunoluiz/xpdig/internal/bubbles/component/modal"
	"github.com/brunoluiz/xpdig/internal/bubbles/component/navigator"
	"github.com/brunoluiz/xpdig/internal/xplane"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// When the modal is visible, route all input to it
	if m.modal.Visible {
		var modalCmd tea.Cmd
		m.modal, modalCmd = m.modal.Update(msg)
		return m, modalCmd
	}

	var cmd tea.Cmd
	switch msg := msg.(type) {
	case *xplane.Resource:
		cmd = m.onCrossplaneUpdate(msg)
	case tea.WindowSizeMsg:
		return m, m.onResize(msg)
	case tea.KeyMsg:
		cmd = m.onKey(msg)
	case navigator.EventItemFocused:
		m.statusbar.SetPath(m.pathByData[msg.ID])
	case navigator.EventItemStatus:
		m.onStatusModal(msg)
		return m, nil
	case navigator.EventToggleUsage:
		m.showUsageDetails = !m.showUsageDetails
		if m.lastTrace != nil {
			m.setData(m.lastTrace)
		}
		return m, nil
	case modal.EventClosed:
		// modal already hidden by its own Update
	}

	if !m.ready {
		var spinnerCmd tea.Cmd
		m.spinner, spinnerCmd = m.spinner.Update(msg)
		return m, spinnerCmd
	}

	var navigatorCmd tea.Cmd
	m.navigator, navigatorCmd = m.navigator.Update(msg)

	var statusBarCmd tea.Cmd
	m.statusbar, statusBarCmd = m.statusbar.Update(msg)

	return m, tea.Batch(cmd, navigatorCmd, statusBarCmd)
}

func (m *Model) onCrossplaneUpdate(data *xplane.Resource) tea.Cmd {
	if data == nil {
		return nil
	}

	m.setColumns(data.Unstructured.GroupVersionKind().GroupKind())
	m.setData(data)

	if m.watch {
		return tea.Tick(m.watchInterval, func(_ time.Time) tea.Msg {
			return m.getTrace()()
		})
	}
	return nil
}

func (m *Model) onResize(msg tea.WindowSizeMsg) tea.Cmd {
	var navigatorCmd, statusbarCmd tea.Cmd
	m.width = msg.Width
	m.height = msg.Height

	top, _, _, _ := lipgloss.NewStyle().Padding(1).GetPadding()
	m.navigator, navigatorCmd = m.navigator.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height - top})

	m.statusbar, statusbarCmd = m.statusbar.Update(msg)
	m.modal.SetSize(m.width, m.height)

	return tea.Batch(navigatorCmd, statusbarCmd)
}

func (m *Model) onKey(_ tea.KeyMsg) tea.Cmd {
	return nil
}

func (m *Model) onStatusModal(msg navigator.EventItemStatus) {
	trace, ok := msg.Data.(*xplane.Resource)
	if !ok {
		return
	}

	gk := trace.Unstructured.GroupVersionKind().GroupKind()
	name := fmt.Sprintf("%s/%s", trace.Unstructured.GetKind(), trace.Unstructured.GetName())

	var status string
	if xplane.IsPkg(gk) {
		resStatus := xplane.GetPkgResourceStatus(trace, name)
		status = fmt.Sprintf(
			"Name:      %s\nPackage:   %s\nVersion:   %s\nInstalled: %s\nHealthy:   %s\nState:     %s\nStatus:    %s",
			resStatus.Name, resStatus.PackageImg, resStatus.Version,
			resStatus.Installed, resStatus.Healthy, resStatus.State, resStatus.Status,
		)
	} else {
		resStatus := xplane.GetResourceStatus(trace, name)
		status = fmt.Sprintf(
			"Name:     %s\nResource: %s\nSynced:   %s\nReady:    %s\nStatus:   %s",
			resStatus.Name, resStatus.ResourceName,
			resStatus.Synced, resStatus.Ready, resStatus.Status,
		)
	}

	m.modal.SetSize(m.width, m.height)
	m.modal.Show("Resource Status", status)
}
