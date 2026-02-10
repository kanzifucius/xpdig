package xpnavigator

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/brunoluiz/xpdig/internal/bubbles/component/modal"
	"github.com/brunoluiz/xpdig/internal/bubbles/component/navigator"
	"github.com/brunoluiz/xpdig/internal/bubbles/component/statusbar"
	"github.com/brunoluiz/xpdig/internal/bubbles/component/table"
	"github.com/brunoluiz/xpdig/internal/xplane"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	HeaderKeyObject = "OBJECT"

	HeaderKeyVersion       = "VERSION"
	HeaderKeyInstalled     = "INSTALLED"
	HeaderKeyInstalledLast = "INSTALLED LAST"
	HeaderKeyHealthy       = "HEALTHY"
	HeaderKeyHealthyLast   = "HEALTHY LAST"
	HeaderKeyState         = "STATE"

	HeaderKeyGroup      = "GROUP"
	HeaderKeyResource   = "RESOURCE"
	HeaderKeySynced     = "SYNCED"
	HeaderKeySyncedLast = "SYNCED LAST"
	HeaderKeyReady      = "READY"
	HeaderKeyReadyLast  = "READY LAST"

	HeaderKeyStatus = "STATUS"
)

type Tracer interface {
	GetTrace() (*xplane.Resource, error)
}

type Model struct {
	keyMap        KeyMap
	navigator     navigator.Model
	statusbar     statusbar.Model
	modal         modal.Model
	tracer        Tracer
	width         int
	height        int
	short         bool
	watch         bool
	watchInterval time.Duration
	logger        *slog.Logger
	ready         bool
	spinner       spinner.Model

	kind       schema.GroupKind
	pathByData map[string][]string

	// usageData holds Usage association data for the current trace tree.
	usageData        xplane.UsageData
	showUsageDetails bool // toggle for virtual sub-rows and markers

	// lastTrace stores the last trace for re-rendering on toggle
	lastTrace *xplane.Resource
}

type WithOpt func(*Model)

func WithWatch(enabled bool) func(*Model) {
	return func(m *Model) {
		m.watch = enabled
	}
}

func WithWatchInterval(t time.Duration) func(*Model) {
	return func(m *Model) {
		m.watchInterval = t
	}
}

func WithShortColumns(enabled bool) func(*Model) {
	return func(m *Model) {
		m.short = enabled
	}
}

func New(
	logger *slog.Logger,
	navModel navigator.Model,
	statusModel statusbar.Model,
	tracer Tracer,
	opts ...WithOpt,
) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := Model{
		keyMap:        DefaultKeyMap(),
		logger:        logger,
		navigator:     navModel,
		statusbar:     statusModel,
		modal:         modal.New(),
		tracer:        tracer,
		width:         0,
		height:        0,
		watchInterval: 10 * time.Second,
		short:         true,
		pathByData:    map[string][]string{},
		ready:         false,
		spinner:       s,
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
}

func (m Model) getTrace() tea.Cmd {
	return func() tea.Msg {
		res, err := m.tracer.GetTrace()
		if err != nil {
			return err
		}
		return res
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.getTrace(), m.spinner.Tick)
}

func (m Model) View() string {
	if !m.ready {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			lipgloss.JoinHorizontal(lipgloss.Left, m.spinner.View(), " Loading..."),
		)
	}

	base := lipgloss.JoinVertical(
		lipgloss.Left,
		m.navigator.View(),
		m.statusbar.View(),
	)

	if m.modal.Visible {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.modal.View(),
		)
	}

	return base
}

type ColumnLayout int

const (
	UnknownColumnLayout ColumnLayout = iota
	ShortObjectColumnLayout
	WideObjectColumnLayout
	ShortPkgColumnLayout
	WidePkgColumnLayout
)

func (m Model) getColumns(layout ColumnLayout) []table.Column {
	switch layout {
	case ShortObjectColumnLayout:
		return []table.Column{
			{Title: HeaderKeyObject, Width: 55},
			{Title: HeaderKeyGroup, Width: 25},
			{Title: HeaderKeyResource, Width: 20},
			{Title: HeaderKeySynced, Width: 7},
			{Title: HeaderKeyReady, Width: 7},
			{Title: HeaderKeyStatus, Width: 68},
		}
	case WideObjectColumnLayout:
		return []table.Column{
			{Title: HeaderKeyObject, Width: 55},
			{Title: HeaderKeyGroup, Width: 25},
			{Title: HeaderKeyResource, Width: 20},
			{Title: HeaderKeySynced, Width: 7},
			{Title: HeaderKeySyncedLast, Width: 19},
			{Title: HeaderKeyReady, Width: 7},
			{Title: HeaderKeyReadyLast, Width: 19},
			{Title: HeaderKeyStatus, Width: 68},
		}
	case ShortPkgColumnLayout:
		return []table.Column{
			{Title: HeaderKeyObject, Width: 60},
			{Title: HeaderKeyVersion, Width: 8},
			{Title: HeaderKeyInstalled, Width: 8},
			{Title: HeaderKeyHealthy, Width: 7},
			{Title: HeaderKeyState, Width: 7},
			{Title: HeaderKeyStatus, Width: 68},
		}
	case WidePkgColumnLayout:
		return []table.Column{
			{Title: HeaderKeyObject, Width: 60},
			{Title: HeaderKeyVersion, Width: 8},
			{Title: HeaderKeyInstalled, Width: 10},
			{Title: HeaderKeyInstalledLast, Width: 19},
			{Title: HeaderKeyHealthy, Width: 7},
			{Title: HeaderKeyHealthyLast, Width: 19},
			{Title: HeaderKeyState, Width: 7},
			{Title: HeaderKeyStatus, Width: 68},
		}
	default:
		return []table.Column{}
	}
}

func (m Model) getLayout(gk schema.GroupKind) ColumnLayout {
	isPkg := xplane.IsPkg(gk)
	isRes, isWide, isShort := !isPkg, !m.short, m.short

	switch {
	case isPkg && isShort:
		return ShortPkgColumnLayout
	case isPkg && isWide:
		return WidePkgColumnLayout
	case isRes && isShort:
		return ShortObjectColumnLayout
	case isRes && isWide:
		return WideObjectColumnLayout
	default:
		return UnknownColumnLayout
	}
}

func (m *Model) setColumns(gk schema.GroupKind) {
	m.navigator.SetColumns(m.getColumns(m.getLayout(gk)))
}

func (m *Model) setData(data *xplane.Resource) {
	m.ready = true
	m.lastTrace = data
	rows := []navigator.DataRow{}
	m.kind = data.Unstructured.GroupVersionKind().GroupKind()
	m.usageData = xplane.CollectUsageData(data)
	m.traceToRows(data, &rows, 0, []string{}, []bool{})
	m.navigator.SetData(rows)
}

func (m Model) traceToRows(v *xplane.Resource, rows *[]navigator.DataRow, depth int, currentPath []string, isLastChilds []bool) {
	name := fmt.Sprintf("%s/%s", v.Unstructured.GetKind(), v.Unstructured.GetName())
	group := v.Unstructured.GetObjectKind().GroupVersionKind().Group
	rowID := fmt.Sprintf("%s.%s/%s", v.Unstructured.GetKind(), group, v.Unstructured.GetName())
	row := navigator.DataRow{
		ID:      rowID,
		Data:    v,
		Columns: []string{},
	}

	// Build tree prefix (standard connectors)
	var prefix string
	if depth > 0 {
		for i := 0; i < depth-1; i++ {
			if isLastChilds[i] {
				prefix += "   "
			} else {
				prefix += "│  "
			}
		}
		if len(isLastChilds) > 0 && isLastChilds[depth-1] {
			prefix += "└─ "
		} else {
			prefix += "├─ "
		}
	}

	label := prefix + name

	// Check if this row is a target of a Usage — add a cyan diamond marker (only when toggle is on)
	if m.showUsageDetails {
		if _, ok := m.usageData.TargetUsageLabels[rowID]; ok {
			row.ConnectorSuffix = " ◆"
			row.ConnectorSuffixColor = lipgloss.Color("#00D7FF")
		}
	}

	paused := false
	if v.Unstructured.GetAnnotations()["crossplane.io/paused"] == "true" {
		label += " (paused)"
		row.Color = lipgloss.ANSIColor(ansi.Yellow)
		paused = true
	}

	var data map[string]string
	if xplane.IsPkg(m.kind) {
		resStatus := xplane.GetPkgResourceStatus(v, label)
		data = map[string]string{
			HeaderKeyObject:        label,
			HeaderKeyGroup:         group,
			HeaderKeyVersion:       resStatus.Version,
			HeaderKeyInstalled:     resStatus.Installed,
			HeaderKeyInstalledLast: getTimeStr(resStatus.InstalledLastTransition),
			HeaderKeyHealthy:       resStatus.Healthy,
			HeaderKeyHealthyLast:   getTimeStr(resStatus.HealthyLastTransition),
			HeaderKeyState:         resStatus.State,
			HeaderKeyStatus:        resStatus.Status,
		}
		if !paused {
			switch {
			case resStatus.Ok:
				row.Color = lipgloss.Color("#39FF14")
			default:
				row.Color = lipgloss.ANSIColor(ansi.Red)
			}
		}
	} else {
		resStatus := xplane.GetResourceStatus(v, label)
		data = map[string]string{
			HeaderKeyObject:     label,
			HeaderKeyGroup:      group,
			HeaderKeyResource:   resStatus.ResourceName,
			HeaderKeySynced:     resStatus.Synced,
			HeaderKeySyncedLast: getTimeStr(resStatus.SyncedLastTransition),
			HeaderKeyReady:      resStatus.Ready,
			HeaderKeyReadyLast:  getTimeStr(resStatus.ReadyLastTransition),
			HeaderKeyStatus:     resStatus.Status,
		}
		// NOTE: in cases where a resource relies on auto-ready, such as kubernetes MRs in Crossplane v2, the synced/ready will
		// always be "-". To avoid it to be shown in red, the second conditional has been added.
		if !paused {
			switch {
			case resStatus.Ok:
				row.Color = lipgloss.Color("#39FF14")
			case !resStatus.Ok && (resStatus.HasSyncedCondition || resStatus.Ready != "-"):
				row.Color = lipgloss.ANSIColor(ansi.Red)
			}
		}
	}

	for _, col := range m.getColumns(m.getLayout(m.kind)) {
		row.Columns = append(row.Columns, data[col.Title])
	}
	*rows = append(*rows, row)

	// Index current path
	path := make([]string, len(currentPath))
	copy(path, currentPath)
	path = append(path, name)
	m.pathByData[row.ID] = path

	// If this is a Usage resource and usage details are toggled on, inject virtual sub-rows
	if m.showUsageDetails {
		if assoc := xplane.GetUsageAssociation(v); assoc != nil {
			m.addUsageVirtualRows(v, rows, depth, isLastChilds, assoc)
		}
	}

	// Recursively process children
	for i, cv := range v.Children {
		last := i == len(v.Children)-1
		m.traceToRows(cv, rows, depth+1, path, append(isLastChilds, last))
	}
}

// addUsageVirtualRows adds display-only sub-rows under a Usage node showing
// the "by" and "of" resource references with cyan styling.
func (m Model) addUsageVirtualRows(v *xplane.Resource, rows *[]navigator.DataRow, depth int, isLastChilds []bool, assoc *xplane.UsageAssociation) {
	// Build the indentation for virtual rows (one level deeper than the Usage)
	var baseIndent string
	if depth > 0 {
		for i := 0; i < depth; i++ {
			if isLastChilds[i] {
				baseIndent += "   "
			} else {
				baseIndent += "│  "
			}
		}
	}

	hasChildren := len(v.Children) > 0

	// Collect virtual entries
	type virtualEntry struct {
		role  string // "by" or "of"
		label string
	}
	var entries []virtualEntry
	if assoc.ByLabel != "" {
		entries = append(entries, virtualEntry{role: "by", label: assoc.ByLabel})
	}
	if assoc.OfLabel != "" {
		entries = append(entries, virtualEntry{role: "of", label: assoc.OfLabel})
	}

	for i, entry := range entries {
		isLast := i == len(entries)-1 && !hasChildren
		var connector string
		if isLast {
			connector = "└─ "
		} else {
			connector = "├─ "
		}

		virtualLabel := baseIndent + connector + entry.role + ": " + entry.label

		virtualRow := navigator.DataRow{
			ID:   assoc.UsageID + "/" + entry.role,
			Data: v, // Point back to the Usage resource for interactions
		}
		virtualRow.ConnectorPrefix = virtualLabel
		virtualRow.ConnectorColor = lipgloss.Color("#00D7FF")

		// Fill columns — only the OBJECT column has content
		for _, col := range m.getColumns(m.getLayout(m.kind)) {
			if col.Title == HeaderKeyObject {
				virtualRow.Columns = append(virtualRow.Columns, "")
			} else {
				virtualRow.Columns = append(virtualRow.Columns, "")
			}
		}
		*rows = append(*rows, virtualRow)
	}
}

func getTimeStr(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC822)
}
