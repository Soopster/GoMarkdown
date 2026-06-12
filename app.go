package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/fsnotify/fsnotify"
)

// Compiled regexes for performance (avoid recompilation)
var (
	reHeading       = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reHeadingTight  = regexp.MustCompile(`^(#{1,6})(\S.*)$`)
	reListTask      = regexp.MustCompile(`^(\s*)([-+*])\s+\[([ xX])\]\s*(.*)$`)
	reListUnordered = regexp.MustCompile(`^(\s*)([-+*])\s+(.*)$`)
	reListOrdered   = regexp.MustCompile(`^(\s*)(\d+)([.)])\s+(.*)$`)
	bgRewriterCache sync.Map
	// builderPool recycles *strings.Builder across scrollbar rows and frames,
	// avoiding repeated heap allocation of the backing []byte buffer.
	builderPool = sync.Pool{New: func() any { return new(strings.Builder) }}
)

var autoPairClosers = map[rune]rune{
	'(':  ')',
	'[':  ']',
	'{':  '}',
	'"':  '"',
	'\'': '\'',
	'`':  '`',
}

type backgroundRewriter struct {
	seq      string
	replacer *strings.Replacer
}

func getBackgroundRewriter(bg string) *backgroundRewriter {
	if cached, ok := bgRewriterCache.Load(bg); ok {
		if rw, ok := cached.(*backgroundRewriter); ok {
			return rw
		}
	}
	seq := backgroundSGR(bg)
	rw := &backgroundRewriter{
		seq: seq,
		replacer: strings.NewReplacer(
			"\x1b[0m", "\x1b[0m"+seq,
			"\x1b[m", "\x1b[m"+seq,
			"\x1b[49m", "\x1b[49m"+seq,
		),
	}
	bgRewriterCache.Store(bg, rw)
	return rw
}

type viewMode int

const (
	modePreview viewMode = iota
	modeRaw
	modeSplit
)

type scrollbarDragTarget int

const (
	scrollbarDragNone scrollbarDragTarget = iota
	scrollbarDragPreview
	scrollbarDragRaw
)

type perfVisualMode int

const (
	perfVisualAuto perfVisualMode = iota
	perfVisualForceOn
	perfVisualForceOff
)

const defaultStyleName = "default"
const catppuccinStyle = "catppuccin"
const nordStyle = "nord"
const gruvboxStyle = "gruvbox"
const solarizedDarkStyle = "solarized-dark"
const solarizedLightStyle = "solarized-light"
const everforestStyle = "everforest"
const kanagawaStyle = "kanagawa"
const sessionFileName = ".markdownviewer.session.json"
const sessionVersion = 1
const largeDocLineThreshold = 2500
const defaultMaxFPS = 60

const (
	layerIDHelpOverlay  = "overlay.help"
	layerIDThemeOverlay = "overlay.theme"
	layerIDCmdOverlay   = "overlay.command"
)

// Scroll physics constants
const (
	mouseWheelScrollDelta = 0.25
	momentumTickInterval  = 16 * time.Millisecond
)

const (
	mouseWheelBoundaryNone = 0
	mouseWheelBoundaryUp   = -1
	mouseWheelBoundaryDown = 1
)

// Debounce and timing constants
const (
	searchDebounceDelay   = 200 * time.Millisecond
	contentSearchDebounce = 300 * time.Millisecond
	autoSaveDelay         = 2 * time.Second
)

// Layout constants
const (
	defaultListRatio = 0.33
	noWrapWidth      = 10000
)

type colorPalette struct {
	bg        string
	surface   string
	border    string
	text      string
	muted     string
	warn      string
	highlight string
}

var themeOrder = []string{
	defaultStyleName,
	styles.DraculaStyle,
	styles.DarkStyle,
	styles.LightStyle,
	styles.TokyoNightStyle,
	styles.PinkStyle,
	catppuccinStyle,
	nordStyle,
	gruvboxStyle,
	solarizedDarkStyle,
	solarizedLightStyle,
	everforestStyle,
	kanagawaStyle,
}

var markdownExts = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".mdown":    {},
	".mkd":      {},
	".mdx":      {},
}

type navigatorNodeKind int

const (
	navigatorDirNode navigatorNodeKind = iota
	navigatorFileNode
)

type navigatorNode struct {
	kind     navigatorNodeKind
	name     string
	path     string
	modTime  time.Time
	children []*navigatorNode
}

type navigatorItem struct {
	kind     navigatorNodeKind
	name     string
	path     string
	depth    int
	expanded bool
	relDir   string
	modTime  time.Time
}

func (f navigatorItem) Title() string {
	indent := strings.Repeat("  ", f.depth)
	marker := "  "
	label := f.name
	if f.kind == navigatorDirNode {
		if f.expanded {
			marker = "▾ "
		} else {
			marker = "▸ "
		}
		label += "/"
	} else if f.relDir != "" {
		label = fmt.Sprintf("%s  %s", label, f.relDir)
	}
	return indent + marker + label
}

func (f navigatorItem) Description() string { return "" }

func (f navigatorItem) FilterValue() string {
	if f.relDir == "" {
		return f.name
	}
	return f.name + " " + f.relDir
}

type filesRefreshedMsg struct {
	root          *navigatorNode
	err           error
	reloadCurrent bool
}

type fileLoadedMsg struct {
	path    string
	content string
	err     error
}

type renderedMsg struct {
	content     string
	err         error
	yOffset     int
	width       int
	renderMs    float64
	generation  int
	annotated   string
	cacheKey    string
	lineNums    bool
	readingMode bool
	richPreview bool
	codePlain   bool
	styleName   string
}

type fileSavedMsg struct {
	path string
	err  error
}

type fileCreatedMsg struct {
	path string
	err  error
}

type fileRenamedMsg struct {
	oldPath string
	newPath string
	err     error
}

type fileDeletedMsg struct {
	path string
	err  error
}

type fsEventMsg struct {
	event fsnotify.Event
}

type fsWatchErrMsg struct{ err error }

type autoReloadClearMsg struct{}
type toastClearMsg struct{}
type searchDebounceMsg struct{ generation int }
type scrollTickMsg struct{ generation int }
type keyboardHoldTickMsg struct{ generation int }
type boundaryFlashClearMsg struct{ generation int }
type editFocusLineClearMsg struct{ generation int }
type autoSaveTickMsg struct{ generation int }
type splitRenderDebounceMsg struct{ generation int }
type externalReloadMsg struct {
	generation int
	path       string
}
type walkDirResultMsg struct{ files []string }
type contentSearchResultMsg struct {
	hits       []contentSearchHit
	generation int
}
type contentSearchDebounceMsg struct{ generation int }
type overlayLayerHitMsg struct {
	id    string
	mouse tea.MouseMsg
}

type gitStatusMsg struct {
	status map[string]string
}

type docStats struct {
	words       int
	chars       int
	paragraphs  int
	headings    int
	readingMins float64
}

func computeDocStats(content string, headingCount int) docStats {
	chars := utf8.RuneCountInString(content)
	words := 0
	paragraphs := 0
	inParagraph := false
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inParagraph {
				paragraphs++
				inParagraph = false
			}
			continue
		}
		inParagraph = true
		// Count words: split by whitespace
		fields := strings.Fields(trimmed)
		words += len(fields)
	}
	if inParagraph {
		paragraphs++
	}
	readingMins := float64(words) / 200.0
	return docStats{
		words:       words,
		chars:       chars,
		paragraphs:  paragraphs,
		headings:    headingCount,
		readingMins: readingMins,
	}
}

type navEntry struct {
	path    string
	yOffset int
	heading int
}

type bookmarkEntry struct {
	Path    string `json:"path"`
	YOffset int    `json:"y_offset"`
	Line    int    `json:"line"`
	Heading int    `json:"heading"`
}

type fileFinderEntry struct {
	path       string
	score      int
	matchRunes []int // indices of matched characters for highlighting
}

type contentSearchHit struct {
	path       string
	line       int
	context    string
	matchStart int
	matchEnd   int
}

type headingItem struct {
	title      string
	level      int
	line       int
	display    string
	renderLine int
}

type searchHit struct {
	line  int
	start int
}

type searchOptions struct {
	caseSensitive bool
	wholeWord     bool
}

type opMode int

const (
	opNone opMode = iota
	opCreate
	opRename
	opDeleteConfirm
	opGoToLine
	opGoToHeading
)

type clipboardTarget int

const (
	clipboardTargetNone clipboardTarget = iota
	clipboardTargetSearch
	clipboardTargetReplace
	clipboardTargetPrompt
	clipboardTargetCmdPalette
	clipboardTargetEditor
)

func (h headingItem) Title() string {
	if h.display != "" {
		return h.display
	}
	return strings.Repeat("  ", max(0, h.level-1)) + h.title
}
func (h headingItem) Description() string { return fmt.Sprintf("Line %d", h.line+1) }
func (h headingItem) FilterValue() string { return h.title }

type sessionState struct {
	Version            int                      `json:"version"`
	CurrentPath        string                   `json:"current_path"`
	Mode               string                   `json:"mode"`
	PreviewYOffset     int                      `json:"preview_y_offset"`
	ListWidthRatio     float64                  `json:"list_width_ratio"`
	ShowOutline        bool                     `json:"show_outline"`
	FullScreen         bool                     `json:"full_screen"`
	FocusRight         bool                     `json:"focus_right"`
	RichPreview        bool                     `json:"rich_preview"`
	StyleName          string                   `json:"style_name"`
	FollowSystem       bool                     `json:"follow_system"`
	Zoom               float64                  `json:"zoom"`
	ShowLineNums       bool                     `json:"show_line_nums"`
	FocusMode          bool                     `json:"focus_mode"`
	ReadingMode        bool                     `json:"reading_mode"`
	CodePlain          bool                     `json:"code_plain"`
	ReducedMotion      bool                     `json:"reduced_motion"`
	ShowGauge          bool                     `json:"show_gauge"`
	PerfVisualMode     string                   `json:"perf_visual_mode"`
	CurrentHeading     int                      `json:"current_heading"`
	FormatOnSave       bool                     `json:"format_on_save"`
	EditHomeEndWrapped bool                     `json:"edit_home_end_wrapped"`
	ShowStats          bool                     `json:"show_stats,omitempty"`
	AutoSave           bool                     `json:"auto_save,omitempty"`
	EditSoftWrap       bool                     `json:"edit_soft_wrap,omitempty"`
	Bookmarks          map[string]bookmarkEntry `json:"bookmarks,omitempty"`
	FoldedSections     []int                    `json:"folded_sections,omitempty"`
}

type layoutCache struct {
	// Full frame cache for no-op updates such as fractional wheel accumulation.
	frameContent string
	frameOnMouse func(tea.MouseMsg) tea.Cmd
	// Left panel cache
	leftPanel      string
	leftOutline    bool
	leftFocusRight bool
	leftOutIdx     int
	leftHeadingCnt int
	leftFileIdx    int
	leftFileCnt    int
	leftWidth      int
	leftHeight     int
	leftBg         string
	// Breadcrumb cache
	breadcrumbStr    string
	breadcrumbHdg    int
	breadcrumbHdgCnt int
	breadcrumbWidth  int
	breadcrumbPath   string
	breadcrumbBg     string
	// Status bar cache
	statusStr     string
	statusMsg     string
	statusCrumb   string
	statusMode    viewMode
	statusOutline bool
	statusFocusR  bool
	statusFullScr bool
	statusToast   string
	statusAutoRld bool
	statusWidth   int
	statusBg      string
	statusEditRow int
	statusEditCol int
	statusSelCh   int
	statusSelLn   int
	// Scrollbar slice reuse
	scrollRows   []string
	scrollWidths []int
	// Divider cache (keyed on palette bg)
	dividerStr string
	dividerBg  string
	// Quick actions cache
	quickStr         string
	quickMode        viewMode
	quickFocusR      bool
	quickOutline     bool
	quickShowSearch  bool
	quickOpMode      opMode
	quickHasHeadings bool
	quickHasPath     bool
	quickWidth       int
	quickBg          string
	// Right pane border cache (top/bottom change only on resize, focus, or theme change)
	rightBorderTop    string
	rightBorderBottom string
	rightBorderLeft   string
	rightBorderRight  string
	rightBorderActive bool
	rightBorderWidth  int
	rightBorderBg     string
	rightBorderLabel  string
}

type perfState struct {
	enabled bool
	overlay bool
	seq     uint64

	viewLastMs   float64
	viewAvgMs    float64
	layoutLastMs float64
	layoutAvgMs  float64
	renderLastMs float64
	renderAvgMs  float64

	fps              float64
	frameCount       int
	frameWindowStart time.Time

	logPath  string
	logFile  *os.File
	logEvery uint64
}

type editUndoEntry struct {
	content string
	row     int
	col     int
}

type editUndoCoalesceKind uint8

const (
	editUndoCoalesceNone editUndoCoalesceKind = iota
	editUndoCoalesceInsert
	editUndoCoalesceDeleteBackward
	editUndoCoalesceDeleteForward
	editUndoCoalesceTransform
)

type momentumAxis uint8

const (
	momentumAxisNone momentumAxis = iota
	momentumAxisPreviewVertical
	momentumAxisEditVertical
	momentumAxisEditHorizontal
)

type momentumProfile struct {
	initialImpulse    float64
	initialStep       float64
	initialMaxVel     float64
	repeatImpulseBase float64
	repeatImpulseRamp float64
	repeatStepBase    float64
	repeatStepRamp    float64
	repeatMaxVelBase  float64
	repeatMaxVelRamp  float64
	startThreshold    float64
	stopThreshold     float64
	heldFriction      float64
	releaseFriction   float64
}

var previewMomentumProfile = momentumProfile{
	initialImpulse:    0.18,
	initialStep:       1.0,
	initialMaxVel:     2.6,
	repeatImpulseBase: 0.16,
	repeatImpulseRamp: 0.07,
	repeatStepBase:    0.45,
	repeatStepRamp:    0.22,
	repeatMaxVelBase:  2.6,
	repeatMaxVelRamp:  2.0,
	startThreshold:    0.14,
	stopThreshold:     0.08,
	heldFriction:      0.94,
	releaseFriction:   0.90,
}

var editVerticalMomentumProfile = momentumProfile{
	initialImpulse:    0,
	initialStep:       1.0,
	initialMaxVel:     2.6,
	repeatImpulseBase: 0.16,
	repeatImpulseRamp: 0.07,
	repeatStepBase:    1.0,
	repeatStepRamp:    0.20,
	repeatMaxVelBase:  2.6,
	repeatMaxVelRamp:  2.0,
	startThreshold:    0.14,
	stopThreshold:     0.08,
	heldFriction:      0.94,
	releaseFriction:   0.90,
}

var editHorizontalMomentumProfile = momentumProfile{
	initialImpulse:    0,
	initialStep:       1.0,
	initialMaxVel:     2.2,
	repeatImpulseBase: 0.20,
	repeatImpulseRamp: 0.08,
	repeatStepBase:    1.0,
	repeatStepRamp:    0.16,
	repeatMaxVelBase:  2.2,
	repeatMaxVelRamp:  1.6,
	startThreshold:    0.12,
	stopThreshold:     0.08,
	heldFriction:      0.94,
	releaseFriction:   0.90,
}

type model struct {
	dir                   string
	fileList              list.Model
	fileTree              *navigatorNode
	fileTreeExpanded      map[string]bool
	outline               list.Model
	fileDelegate          list.DefaultDelegate
	outDelegate           list.DefaultDelegate
	viewport              viewport.Model
	textarea              textarea.Model
	sourceContent         string
	mode                  viewMode
	status                string
	currentPath           string
	loadedPath            string
	width                 int
	height                int
	listWidthRatio        float64
	isDragging            bool
	showThemePicker       bool
	themePickerIdx        int
	followSystem          bool
	zoom                  float64
	searchInput           textinput.Model
	replaceInput          textinput.Model
	replaceInputFocus     bool
	promptInput           textinput.Model
	searchMatches         []searchHit
	searchIndex           int
	showSearch            bool
	showReplace           bool
	replaceCaseSensitive  bool
	replaceWholeWord      bool
	replaceScopeSelection bool
	searchReturnRow       int
	searchReturnCol       int
	searchReturnSet       bool
	searchQueryLen        int
	searchGeneration      int
	renderGeneration      int
	opMode                opMode
	opTarget              string
	pendingDelete         string
	pendingRename         string
	pendingNew            bool
	toast                 string
	toastUntil            time.Time
	watcher               *fsnotify.Watcher
	richPreview           bool
	autoReloadAt          time.Time
	showAutoReload        bool
	externalReloadGen     int
	styleName             string
	showOutline           bool
	showLineNums          bool
	rendered              string
	renderedLineCache     []string // split lines of rendered; invalidated when rendered changes
	previewYOffset        int
	mouseWheelAccum       float64
	mouseWheelBoundaryDir int
	reuseLastFrame        bool
	scrollAccum           float64
	scrollVelocity        float64
	scrollMomentum        bool
	scrollMomentumGen     int
	scrollMomentumAxis    momentumAxis
	// Clipboard read responses are async; this stores where incoming content goes.
	pendingClipboardTarget clipboardTarget
	// Boundary flash (+1=top, -1=bottom)
	boundaryFlash        int
	boundaryFlashGen     int
	terminalFocused      bool
	focusRight           bool
	showHelp             bool
	fullScreen           bool
	reducedMotion        bool
	formatOnSave         bool
	editHomeEndWrapped   bool
	colorProfile         colorprofile.Profile
	colorProfileKnown    bool
	termCapRGB           bool
	termCapTc            bool
	terminalBgKnown      bool
	terminalBgDark       bool
	terminalBgColor      string
	terminalFgColor      string
	terminalCursorColor  string
	capProbeRequested    bool
	scrollKeyHeld        bool // true while a scroll key is actively repeating (native or inferred fallback)
	keyboardEventTypes   bool // terminal reports key event types (repeat/release) via Bubble Tea v2 enhancements
	lastScrollKeyDir     int  // -1 for up, +1 for down
	lastScrollKeyAt      time.Time
	scrollHoldDir        int
	scrollHoldSince      time.Time
	scrollHoldTickGen    int
	scrollHoldTickActive bool
	sawNativeKeyRepeat   bool
	showGauge            bool
	perfVisualMode       perfVisualMode
	lastPerfAutoDisable  bool
	highlightLine        int
	editFocusLine        int
	editFocusLineGen     int
	currentHeading       int
	headings             []headingItem
	headingRenderLines   []int
	headingRenderIndices []int
	breadcrumb           string
	styles               uiStyles
	palette              colorPalette
	focusMode            bool
	readingMode          bool
	codePlain            bool
	// Command palette
	showCmdPalette   bool
	cmdFilter        string
	cmdIdx           int
	filteredCommands []command
	// Renderer cache
	cachedRenderer    *glamour.TermRenderer
	rendererWidth     int
	rendererStyle     string
	rendererCodePlain bool
	rendererRich      bool
	// annotateCodeBlocks cache
	lastAnnotateContent string
	lastAnnotateWidth   int
	lastAnnotateOutput  string
	// Render output cache (skips Glamour pipeline on cache hit)
	lastRenderCacheKey string
	lastRenderWidth    int
	lastRenderLineNums bool
	lastRenderReading  bool
	lastRenderRich     bool
	lastRenderCode     bool
	lastRenderStyle    string
	lastRenderOutput   string
	// Viewport post-processing cache (skips SetContent when unchanged)
	lastVPRendered  string // Glamour output used
	lastVPHighlight int    // highlight line
	lastVPFocus     bool   // focus mode state
	lastVPReading   bool   // reading mode state
	lastVPHeading   int    // current heading for focus
	lastVPSearchCnt int    // search match count
	lastVPSearchIdx int    // current search index
	sessionPath     string
	restoreSession  bool
	restoreHeading  int
	// Direct viewport line access (bypasses viewport.View() overhead)
	viewportLines      []string
	viewportLineWidths []int
	renderedLineCount  int
	// Layout cache (pointer survives Bubble Tea's value-copy of model)
	layoutCache *layoutCache
	// Mermaid support/cache (pointer survives Bubble Tea's value-copy of model)
	mermaid *mermaidSupport
	// Runtime perf hooks (shared pointer survives Bubble Tea value-copy)
	perf *perfState
	// Edit mode text selection
	editSelActive          bool
	editSelAnchorRow       int
	editSelAnchorCol       int
	editSelAnchorOffset    int
	editSelectExtendActive bool
	editSelectExtendDir    int
	editPreferredCol       int
	editPreferredColSet    bool
	editMouseSelecting     bool
	editMouseAnchorOff     int
	scrollbarDrag          scrollbarDragTarget
	scrollbarDragGrab      int
	editLastClickAt        time.Time
	editLastClickRow       int
	editLastClickCol       int
	editClickCount         int
	// Edit mode undo stack (max 100 entries)
	editUndoStack        []editUndoEntry
	editRedoStack        []editUndoEntry
	editUndoCoalesceKind editUndoCoalesceKind
	// A1: Document statistics
	docStats  docStats
	showStats bool
	// A2: Auto-save
	editDirty   bool
	autoSave    bool
	autoSaveGen int
	// A3: Fuzzy file finder
	showFileFinder    bool
	fileFinderInput   textinput.Model
	fileFinderResults []fileFinderEntry
	fileFinderIdx     int
	fileFinderAll     []string
	// B1: Internal link navigation
	navStack          []navEntry
	pendingLinkAnchor string
	// B2: Live side-by-side preview
	splitEditWidth    int
	splitPreviewWidth int
	splitRenderGen    int
	// B3: Bookmarks/marks
	bookmarks map[rune]bookmarkEntry
	markMode  byte // 0=none, 'm'=set, '\''=jump
	// C1: Section folding
	foldedSections map[int]bool
	// C2: Content search across files
	showContentSearch    bool
	contentSearchInput   textinput.Model
	contentSearchResults []contentSearchHit
	contentSearchIdx     int
	contentSearchGen     int
	// C3: Frontmatter support
	frontmatter      map[string]string
	frontmatterLines int // number of lines stripped
	// C4: Soft wrap toggle
	editSoftWrap bool
	// D1: Table editing mode
	editTableMode  bool
	tableGrid      [][]string
	tableRow       int
	tableCol       int
	tableStartLine int
	tableEndLine   int
	// D2: Multi-buffer (recent files ring)
	recentFiles []string
	// D3: Snippets
	// (no persistent state — snippets are stateless commands)
	// D5: Git integration
	gitFileStatus map[string]string // path -> status char (M, A, ?)
}

func (m *model) invalidateRenderCaches() {
	m.lastAnnotateContent = ""
	m.lastAnnotateWidth = 0
	m.lastAnnotateOutput = ""
	m.lastRenderCacheKey = ""
	m.lastRenderWidth = 0
	m.lastRenderLineNums = false
	m.lastRenderReading = false
	m.lastRenderRich = false
	m.lastRenderCode = false
	m.lastRenderStyle = ""
	m.lastRenderOutput = ""
	m.lastVPRendered = ""
	m.lastVPHighlight = 0
	m.lastVPFocus = false
	m.lastVPReading = false
	m.lastVPHeading = 0
	m.lastVPSearchCnt = 0
	m.lastVPSearchIdx = 0
	if m.layoutCache != nil {
		m.layoutCache.frameContent = ""
		m.layoutCache.frameOnMouse = nil
	}
}

const widthUnknown = -1

const (
	editMultiClickGap       = 450 * time.Millisecond
	editMultiClickTolerance = 1
	editFocusFlashDuration  = 900 * time.Millisecond
)

func (m *model) setViewportContent(content string) {
	m.setViewportContentLines(content, nil)
}

func (m *model) setViewportContentLines(content string, lines []string) {
	m.viewport.SetContent(content)
	// Pre-apply bg rewriting so downstream consumers (viewportVisibleRows,
	// renderWithScrollbarRows, renderPreviewPaneBox) receive lines with the
	// theme background already baked in. This eliminates the per-frame
	// applyGlobalBackground pass in View().
	if m.heavyVisualsEnabled() {
		content = applyGlobalBackground(content, m.palette.bg)
		lines = nil
	}
	switch {
	case lines != nil:
		m.viewportLines = slices.Clone(lines)
	case content != "":
		m.viewportLines = strings.Split(content, "\n")
	default:
		m.viewportLines = []string{""}
	}
	if len(m.viewportLines) == 0 {
		m.viewportLines = []string{""}
	}
	m.renderedLineCount = len(m.viewportLines)
	if cap(m.viewportLineWidths) < m.renderedLineCount {
		m.viewportLineWidths = make([]int, m.renderedLineCount)
	} else {
		m.viewportLineWidths = m.viewportLineWidths[:m.renderedLineCount]
	}
	// Mark all widths as unknown
	for i := range m.viewportLineWidths {
		m.viewportLineWidths[i] = widthUnknown
	}
	// Eagerly compute widths for the initial visible range
	start := m.viewport.YOffset()
	if start < 0 {
		start = 0
	}
	end := start + m.viewport.Height()
	if end > m.renderedLineCount {
		end = m.renderedLineCount
	}
	for i := start; i < end; i++ {
		m.viewportLineWidths[i] = lipgloss.Width(m.viewportLines[i])
	}
}

// viewportVisibleRows returns visible rows and cached row widths.
// Lazily computes widths for visible lines that haven't been measured yet.
func (m model) viewportVisibleRows() ([]string, []int) {
	if len(m.viewportLines) == 0 || m.viewport.Height() <= 0 {
		return nil, nil
	}
	start := m.viewport.YOffset()
	if start < 0 {
		start = 0
	}
	if start >= len(m.viewportLines) {
		start = len(m.viewportLines) - 1
	}
	end := start + m.viewport.Height()
	if end > len(m.viewportLines) {
		end = len(m.viewportLines)
	}
	if start >= end {
		return nil, nil
	}
	rows := m.viewportLines[start:end]
	if len(m.viewportLineWidths) >= end {
		widths := m.viewportLineWidths[start:end]
		// Fill in any unknown widths for visible lines
		for i, w := range widths {
			if w == widthUnknown {
				widths[i] = lipgloss.Width(rows[i])
			}
		}
		return rows, widths
	}
	widths := make([]int, len(rows))
	for i, line := range rows {
		widths[i] = lipgloss.Width(line)
	}
	return rows, widths
}

// triggerBoundaryFlash sets the boundary flash indicator and schedules a clear.
func (m *model) triggerBoundaryFlash(dir int) {
	m.boundaryFlash = dir
	m.boundaryFlashGen++
}

// boundaryFlashCmd returns a tea.Cmd that clears the boundary flash after 900ms.
func (m model) boundaryFlashCmd() tea.Cmd {
	gen := m.boundaryFlashGen
	return tea.Tick(900*time.Millisecond, func(t time.Time) tea.Msg {
		return boundaryFlashClearMsg{generation: gen}
	})
}

// cancelMomentum stops any active momentum scrolling.
func (m *model) cancelMomentum() {
	m.scrollVelocity = 0
	m.scrollMomentum = false
	m.scrollAccum = 0
	m.scrollMomentumAxis = momentumAxisNone
	m.clearKeyboardScrollHold()
	m.lastScrollKeyDir = 0
	m.lastScrollKeyAt = time.Time{}
}

func (m *model) prepareMomentumAxis(axis momentumAxis) {
	if m.scrollMomentumAxis == axis {
		return
	}
	m.scrollVelocity = 0
	m.scrollAccum = 0
	m.scrollMomentum = false
	m.scrollMomentumAxis = axis
}

func (m *model) clearEditSelectionExtend() {
	m.editSelectExtendActive = false
	m.editSelectExtendDir = 0
}

func (m *model) armEditSelectionExtend(dir int) {
	m.editSelectExtendActive = true
	m.editSelectExtendDir = dir
}

func (m *model) editShouldExtendSelection(dir int, msg tea.KeyPressMsg) bool {
	if msg.Mod&tea.ModShift != 0 {
		return true
	}
	return m.editSelActive && m.editSelectExtendActive && m.editSelectExtendDir == dir
}

func editIsSelectionArrowKey(msg tea.KeyPressMsg) bool {
	switch msg.Code {
	case tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight:
		return true
	}
	switch msg.String() {
	case "up", "down", "left", "right", "shift+up", "shift+down", "shift+left", "shift+right":
		return true
	default:
		return false
	}
}

func momentumProfileForAxis(axis momentumAxis) momentumProfile {
	switch axis {
	case momentumAxisEditVertical:
		return editVerticalMomentumProfile
	case momentumAxisEditHorizontal:
		return editHorizontalMomentumProfile
	default:
		return previewMomentumProfile
	}
}

func momentumParamsForAxis(axis momentumAxis, isRepeat bool, holdDuration time.Duration) (impulse, scrollDelta, maxVel float64) {
	profile := momentumProfileForAxis(axis)
	if !isRepeat {
		return profile.initialImpulse, profile.initialStep, profile.initialMaxVel
	}
	holdSec := holdDuration.Seconds()
	if holdSec < 0 {
		holdSec = 0
	}
	if holdSec > 2.5 {
		holdSec = 2.5
	}
	impulse = profile.repeatImpulseBase + (profile.repeatImpulseRamp * holdSec)
	scrollDelta = profile.repeatStepBase + (profile.repeatStepRamp * holdSec)
	maxVel = profile.repeatMaxVelBase + (profile.repeatMaxVelRamp * holdSec)
	if scrollDelta < 1.0 && axis != momentumAxisPreviewVertical {
		scrollDelta = 1.0
	}
	return impulse, scrollDelta, maxVel
}

func momentumTickCmd(generation int) tea.Cmd {
	return tea.Tick(momentumTickInterval, func(t time.Time) tea.Msg {
		return scrollTickMsg{generation: generation}
	})
}

func (m *model) stopMomentum() {
	m.scrollVelocity = 0
	m.scrollMomentum = false
	m.scrollAccum = 0
	m.scrollMomentumAxis = momentumAxisNone
}

func (m *model) stopMomentumWithFlash(dir int) tea.Cmd {
	m.stopMomentum()
	if dir == 0 {
		return nil
	}
	m.triggerBoundaryFlash(dir)
	return m.boundaryFlashCmd()
}

func (m *model) resetMomentumDirection(dir int) {
	if (dir < 0 && m.scrollAccum > 0) || (dir > 0 && m.scrollAccum < 0) {
		m.scrollAccum = 0
	}
	if (dir < 0 && m.scrollVelocity > 0) || (dir > 0 && m.scrollVelocity < 0) {
		m.scrollVelocity = 0
	}
}

func (m *model) applyMomentumDelta(axis momentumAxis, delta int, selecting bool, enforceSelectionDir bool) (blocked bool, flashDir int) {
	switch axis {
	case momentumAxisPreviewVertical:
		prevOffset := m.previewYOffset
		m.viewport.SetYOffset(m.viewport.YOffset() + delta)
		m.previewYOffset = m.viewport.YOffset()
		if m.previewYOffset != prevOffset {
			m.syncCurrentHeading(m.viewport.YOffset())
			return false, 0
		}
		if delta < 0 {
			return true, 1
		}
		return true, -1
	case momentumAxisEditHorizontal:
		if selecting && enforceSelectionDir {
			expectedDir := 2
			if delta < 0 {
				expectedDir = -2
			}
			if m.editSelectExtendDir != expectedDir {
				return true, 0
			}
		}
		if m.scrollTextareaColumns(delta, selecting) {
			return true, 0
		}
		return false, 0
	case momentumAxisEditVertical:
		if selecting && enforceSelectionDir {
			expectedDir := 1
			if delta < 0 {
				expectedDir = -1
			}
			if m.editSelectExtendDir != expectedDir {
				return true, 0
			}
		}
		if m.scrollTextareaLines(delta, selecting) {
			if delta < 0 {
				return true, 1
			}
			return true, -1
		}
		if m.mode == modeSplit {
			m.splitSyncScroll()
		}
		return false, 0
	default:
		return true, 0
	}
}

func (m *model) applyMomentumImpulse(axis momentumAxis, dir int, impulse, scrollDelta, maxVel float64, selecting bool) tea.Cmd {
	m.prepareMomentumAxis(axis)
	m.resetMomentumDirection(dir)
	m.scrollVelocity += float64(dir) * impulse
	if m.scrollVelocity > maxVel {
		m.scrollVelocity = maxVel
	}
	if m.scrollVelocity < -maxVel {
		m.scrollVelocity = -maxVel
	}
	m.scrollAccum += float64(dir) * scrollDelta
	units := int(m.scrollAccum)
	if units != 0 {
		m.scrollAccum -= float64(units)
		if blocked, flashDir := m.applyMomentumDelta(axis, units, selecting, false); blocked {
			return m.stopMomentumWithFlash(flashDir)
		}
	}
	profile := momentumProfileForAxis(axis)
	if !m.scrollMomentum && !m.reducedMotion && math.Abs(m.scrollVelocity) >= profile.startThreshold {
		m.scrollMomentum = true
		m.scrollMomentumGen++
		return momentumTickCmd(m.scrollMomentumGen)
	}
	return nil
}

// applyMomentumScroll applies a single preview momentum impulse from keyboard input.
func (m *model) applyMomentumScroll(isUp bool, impulse, scrollDelta, maxVel float64) tea.Cmd {
	dir := 1
	if isUp {
		dir = -1
	}
	return m.applyMomentumImpulse(momentumAxisPreviewVertical, dir, impulse, scrollDelta, maxVel, false)
}

// scrollTextareaLines moves the textarea cursor by n lines (negative=up, positive=down)
// and returns true if a document boundary was reached (cursor didn't move).
func (m *model) scrollTextareaLines(lines int, selecting bool) bool {
	if lines == 0 {
		return false
	}
	prevLine := m.textarea.Line()
	prevCol := m.editCursorCol()
	step := 1
	if lines < 0 {
		step = -1
		lines = -lines
	}
	for range lines {
		if selecting {
			m.editMoveVertical(step, true)
			continue
		}
		if step < 0 {
			m.textarea.CursorUp()
		} else {
			m.textarea.CursorDown()
		}
	}
	return m.textarea.Line() == prevLine && m.editCursorCol() == prevCol
}

// applyTextareaMomentumScroll applies a momentum impulse that drives textarea cursor
// movement, mirroring preview momentum but with editor-specific movement rules.
func (m *model) applyTextareaMomentumScroll(isUp bool, impulse, scrollDelta, maxVel float64, selecting bool) tea.Cmd {
	dir := 1
	if isUp {
		dir = -1
	}
	return m.applyMomentumImpulse(momentumAxisEditVertical, dir, impulse, scrollDelta, maxVel, selecting)
}

func (m *model) handleMomentumTick(selecting bool) tea.Cmd {
	profile := momentumProfileForAxis(m.scrollMomentumAxis)
	friction := profile.releaseFriction
	if m.scrollKeyHeld {
		friction = profile.heldFriction
	}
	m.scrollVelocity *= friction
	if math.Abs(m.scrollVelocity) < profile.stopThreshold {
		m.stopMomentum()
		return nil
	}
	m.scrollAccum += m.scrollVelocity
	units := int(m.scrollAccum)
	if units == 0 {
		return momentumTickCmd(m.scrollMomentumGen)
	}
	m.scrollAccum -= float64(units)
	if blocked, flashDir := m.applyMomentumDelta(m.scrollMomentumAxis, units, selecting, true); blocked {
		return m.stopMomentumWithFlash(flashDir)
	}
	return momentumTickCmd(m.scrollMomentumGen)
}

func (m *model) applyMouseWheelPreviewScroll(isUp bool) {
	if m.previewWheelAtBoundary(isUp) {
		m.noteMouseWheelBoundary(isUp)
		m.reuseLastFrame = true
		return
	}
	lines := m.consumeMouseWheelLines(isUp)
	if lines == 0 {
		m.reuseLastFrame = true
		return
	}
	prevOffset := m.previewYOffset
	m.cancelMomentum()
	m.reuseLastFrame = false
	m.viewport.SetYOffset(m.viewport.YOffset() + lines)
	m.previewYOffset = m.viewport.YOffset()
	if m.previewYOffset != prevOffset {
		m.clearMouseWheelBoundary()
		m.syncCurrentHeading(m.viewport.YOffset())
		return
	}
	m.noteMouseWheelBoundary(isUp)
	if isUp {
		m.triggerBoundaryFlash(1)
	} else {
		m.triggerBoundaryFlash(-1)
	}
}

func (m *model) applyMouseWheelTextareaScroll(isUp bool, selecting bool) tea.Cmd {
	if m.textareaWheelAtBoundary(isUp) {
		m.noteMouseWheelBoundary(isUp)
		m.reuseLastFrame = true
		return nil
	}
	lines := m.consumeMouseWheelLines(isUp)
	if lines == 0 {
		m.reuseLastFrame = true
		return nil
	}
	m.cancelMomentum()
	m.reuseLastFrame = false
	if m.scrollTextareaLines(lines, selecting) {
		m.noteMouseWheelBoundary(isUp)
		if isUp {
			m.triggerBoundaryFlash(1)
		} else {
			m.triggerBoundaryFlash(-1)
		}
		return m.boundaryFlashCmd()
	}
	m.clearMouseWheelBoundary()
	if m.mode == modeSplit {
		m.splitSyncScroll()
	}
	return nil
}

func (m *model) consumeMouseWheelLines(isUp bool) int {
	if m.mouseWheelBoundaryDir != mouseWheelBoundaryNone {
		if dir := mouseWheelDirection(isUp); dir != m.mouseWheelBoundaryDir {
			m.clearMouseWheelBoundary()
			return dir
		}
	}
	m.clearMouseWheelBoundary()
	delta := mouseWheelScrollDelta
	if isUp {
		delta = -delta
	}
	if (delta > 0 && m.mouseWheelAccum < 0) || (delta < 0 && m.mouseWheelAccum > 0) {
		m.mouseWheelAccum = 0
	}
	m.mouseWheelAccum += delta
	lines := int(m.mouseWheelAccum)
	if lines != 0 {
		m.mouseWheelAccum -= float64(lines)
	}
	return lines
}

func mouseWheelDirection(isUp bool) int {
	if isUp {
		return mouseWheelBoundaryUp
	}
	return mouseWheelBoundaryDown
}

func (m *model) noteMouseWheelBoundary(isUp bool) {
	m.mouseWheelAccum = 0
	m.mouseWheelBoundaryDir = mouseWheelDirection(isUp)
}

func (m *model) clearMouseWheelBoundary() {
	m.mouseWheelBoundaryDir = mouseWheelBoundaryNone
}

func (m model) previewWheelAtBoundary(isUp bool) bool {
	if isUp {
		return m.viewport.AtTop()
	}
	return m.viewport.AtBottom()
}

func (m model) textareaWheelAtBoundary(isUp bool) bool {
	lines := max(1, strings.Count(m.textarea.Value(), "\n")+1)
	if isUp {
		return m.textarea.Line() <= 0
	}
	return m.textarea.Line() >= lines-1
}

// scrollTextareaColumns moves the textarea cursor by n columns
// (negative=left, positive=right) and returns true if a document boundary was
// reached (cursor didn't move).
func (m *model) scrollTextareaColumns(cols int, selecting bool) bool {
	if cols == 0 {
		return false
	}
	prevLine := m.textarea.Line()
	prevCol := m.editCursorCol()
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return true
	}
	step := 1
	if cols < 0 {
		step = -1
		cols = -cols
	}
	for range cols {
		if selecting {
			m.editMoveHorizontal(step, true)
			continue
		}
		row := m.textarea.Line()
		col := m.editCursorCol()
		if step < 0 {
			if col > 0 {
				m.textarea.SetCursorColumn(col - 1)
				continue
			}
			if row > 0 {
				m.textarea.CursorUp()
				m.textarea.CursorEnd()
				continue
			}
			break
		}
		if row >= 0 && row < len(lines) {
			if col < len([]rune(lines[row])) {
				m.textarea.SetCursorColumn(col + 1)
				continue
			}
			if row < len(lines)-1 {
				m.textarea.CursorDown()
				m.textarea.SetCursorColumn(0)
				continue
			}
		}
		break
	}
	return m.textarea.Line() == prevLine && m.editCursorCol() == prevCol
}

func (m *model) applyTextareaHorizontalMomentumScroll(isLeft bool, impulse, scrollDelta, maxVel float64, selecting bool) tea.Cmd {
	dir := 1
	if isLeft {
		dir = -1
	}
	return m.applyMomentumImpulse(momentumAxisEditHorizontal, dir, impulse, scrollDelta, maxVel, selecting)
}

const (
	inferredKeyRepeatWindow = 90 * time.Millisecond
	inferredKeyHoldTimeout  = 120 * time.Millisecond
	keyboardHoldStartDelay  = 170 * time.Millisecond
	keyboardHoldTickEvery   = 34 * time.Millisecond
)

// isKeyboardScrollRepeat returns true for held-key repeat events.
// If terminal event-types are unavailable, it infers repeat from rapid
// same-direction key presses.
func (m *model) isKeyboardScrollRepeat(dir int, nativeRepeat bool) bool {
	now := time.Now()
	if nativeRepeat {
		m.lastScrollKeyAt = now
		m.lastScrollKeyDir = dir
		return true
	}
	// Fallback repeat inference is kept even when event-types are reported, as
	// some terminals provide release events without reliable repeat flags.
	inferred := m.lastScrollKeyDir == dir &&
		!m.lastScrollKeyAt.IsZero() &&
		now.Sub(m.lastScrollKeyAt) <= inferredKeyRepeatWindow
	m.lastScrollKeyAt = now
	m.lastScrollKeyDir = dir
	return inferred
}

func (m *model) clearKeyboardScrollHold() {
	m.scrollKeyHeld = false
	m.scrollHoldDir = 0
	m.scrollHoldSince = time.Time{}
	m.scrollHoldTickActive = false
	m.scrollHoldTickGen++
}

func (m *model) updateKeyboardScrollHold(dir int) time.Duration {
	now := time.Now()
	if !m.scrollKeyHeld || m.scrollHoldDir != dir || m.scrollHoldSince.IsZero() {
		m.scrollHoldSince = now
		m.scrollHoldDir = dir
	}
	m.scrollKeyHeld = true
	return now.Sub(m.scrollHoldSince)
}

func keyboardHoldTickCmd(gen int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return keyboardHoldTickMsg{generation: gen}
	})
}

func (m *model) ensureKeyboardHoldTickLoop() tea.Cmd {
	if m.sawNativeKeyRepeat || !m.scrollKeyHeld || m.scrollHoldDir == 0 || m.scrollHoldTickActive {
		return nil
	}
	m.scrollHoldTickActive = true
	m.scrollHoldTickGen++
	return keyboardHoldTickCmd(m.scrollHoldTickGen, keyboardHoldStartDelay)
}

// decayInferredScrollHold clears inferred key-hold state when no rapid key
// presses have been observed recently.
func (m *model) decayInferredScrollHold() {
	if m.keyboardEventTypes {
		return
	}
	if !m.scrollKeyHeld || m.lastScrollKeyAt.IsZero() {
		return
	}
	if time.Since(m.lastScrollKeyAt) > inferredKeyHoldTimeout {
		m.clearKeyboardScrollHold()
	}
}

type uiStyles struct {
	divider        lipgloss.Style
	dividerVert    string
	status         lipgloss.Style
	statusWarn     lipgloss.Style
	statusChip     lipgloss.Style
	statusChipWarn lipgloss.Style
	title          lipgloss.Style
	pane           lipgloss.Style
	content        lipgloss.Style
	appBg          lipgloss.Style
	// Pre-computed scrollbar styles
	scrollLineBg lipgloss.Style
	scrollTrack  lipgloss.Style
	// Pre-computed heading tree styles
	treeMarkerActive   string // pre-rendered
	treeMarkerInactive string
	treeActive         lipgloss.Style
	treeNormal         lipgloss.Style
	treePrefix         lipgloss.Style
	treeTitle          lipgloss.Style
	// Pre-computed layout border styles
	leftActive    lipgloss.Style
	leftInactive  lipgloss.Style
	rightActive   lipgloss.Style
	rightInactive lipgloss.Style
	// Pre-computed scrollbar background SGR sequence for direct padding
	scrollBgSGR string         // e.g. "\x1b[48;2;30;31;41m" — just the bg color escape
	scrollReset string         // "\x1b[0m"
	scrollFlash lipgloss.Style // boundary flash arrow style
	// Pre-rendered scroll and gauge glyphs for hot paths
	scrollTrackGlyph    string
	scrollThumbGlyph    string
	scrollEmptyGlyph    string
	scrollFlashTop      string
	scrollFlashBottom   string
	gaugeEmpty          string
	gaugeMuted          string
	gaugeActive         string
	gaugeMark           string
	gaugeMetaEmpty      string
	gaugeBoundaryMuted  string
	gaugeBoundaryActive string
	gaugeDepthMuted     [4]string
	gaugeDepthActive    [4]string
	// Pre-computed SGR escape sequences for hot-path rendering
	sgrBgMain      string // background: palette.bg
	sgrBgSurface   string // background: palette.surface
	sgrBgHighlight string // background: palette.highlight
	sgrBgWarn      string // background: palette.warn
	sgrFgText      string // foreground: palette.text
	sgrFgMuted     string // foreground: palette.muted
	sgrFgWarn      string // foreground: palette.warn
	sgrFgBorder    string // foreground: palette.border
	sgrFgBg        string // foreground: palette.bg (for inverted text)
	sgrReset       string // "\x1b[0m"
	sgrBold        string // "\x1b[1m"
	// Composite SGR sequences for common combinations
	sgrDimPrefix    string // muted fg + main bg (for focus dim)
	sgrSearchPri    string // warn bg + bg fg + bold (primary search hit)
	sgrSearchSec    string // highlight bg + text fg (secondary search hit)
	sgrHighlight    string // highlight bg + text fg (heading highlight)
	sgrActiveBorder string // warn fg + highlight bg (active pane border)
	sgrBorderNormal string // border fg + bg bg (normal pane border)
	// Pre-rendered styled strings
	codeBorderStyled string // "▎ " with warn fg + surface bg
	lineNumFmt       string // muted fg + bg bg prefix for line numbers
	lineNumBgWrap    string // bg bg prefix for wrapping line+number
	sgrReadingDim    string // reading focus dim prefix (#3a3a3a fg + bg bg)
	sgrFrameClear    string // bgSeq + "\x1b[K" — appended to every line by padToFrame/clampHeight
	sgrResetBg       string // "\x1b[0m" + bgSeq — reset that preserves theme background
}

func buildStyles(p colorPalette) uiStyles {
	base := lipgloss.Color(p.bg)
	border := lipgloss.Color(p.border)
	text := lipgloss.Color(p.text)
	muted := lipgloss.Color(p.muted)
	warn := lipgloss.Color(p.warn)
	highlight := lipgloss.Color(p.highlight)

	activeBorder := lipgloss.ThickBorder()
	inactiveBorder := lipgloss.RoundedBorder()

	paneBase := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(base).
		Foreground(text)

	return uiStyles{
		divider: lipgloss.NewStyle().
			Foreground(border).
			Background(base),
		dividerVert: lipgloss.NewStyle().
			Foreground(border).
			Background(base).
			Render("│"),
		status: lipgloss.NewStyle().
			Foreground(text).
			Background(base),
		statusWarn: lipgloss.NewStyle().
			Foreground(warn).
			Background(base),
		statusChip: lipgloss.NewStyle().
			Foreground(text).
			Background(highlight).
			Bold(true),
		statusChipWarn: lipgloss.NewStyle().
			Foreground(base).
			Background(warn).
			Bold(true),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(text).
			Background(base),
		pane:    paneBase,
		content: paneBase,
		appBg: lipgloss.NewStyle().
			Background(base).
			Foreground(text),
		// Scrollbar
		scrollLineBg: lipgloss.NewStyle().Background(base),
		scrollTrack: lipgloss.NewStyle().
			Foreground(muted).
			Background(base),
		// Heading tree
		treeMarkerActive:   lipgloss.NewStyle().Background(base).Foreground(warn).Render("▸ "),
		treeMarkerInactive: lipgloss.NewStyle().Background(base).Foreground(muted).Render("  "),
		treeActive:         lipgloss.NewStyle().Background(highlight).Foreground(text).Bold(true),
		treeNormal:         lipgloss.NewStyle().Background(base).Foreground(text),
		treePrefix:         lipgloss.NewStyle().Background(base).Foreground(muted),
		treeTitle:          lipgloss.NewStyle().Background(base).Foreground(text),
		// Layout borders
		leftInactive: paneBase.BorderStyle(inactiveBorder).BorderForeground(border),
		leftActive: paneBase.
			BorderStyle(activeBorder).
			BorderForeground(warn).
			BorderBackground(highlight),
		rightInactive: paneBase.BorderStyle(inactiveBorder).BorderForeground(border),
		rightActive: paneBase.
			BorderStyle(activeBorder).
			BorderForeground(warn).
			BorderBackground(highlight),
		scrollBgSGR: hexToBgSGR(p.bg),
		scrollReset: "\x1b[0m",
		scrollFlash: lipgloss.NewStyle().
			Foreground(warn).
			Background(base).
			Bold(true),
		scrollTrackGlyph: lipgloss.NewStyle().
			Foreground(muted).
			Background(base).
			Render("░"),
		scrollThumbGlyph: lipgloss.NewStyle().
			Foreground(muted).
			Background(base).
			Render("█"),
		scrollEmptyGlyph: lipgloss.NewStyle().
			Background(base).
			Render(" "),
		scrollFlashTop: lipgloss.NewStyle().
			Foreground(warn).
			Background(base).
			Bold(true).
			Render("▲"),
		scrollFlashBottom: lipgloss.NewStyle().
			Foreground(warn).
			Background(base).
			Bold(true).
			Render("▼"),
		gaugeEmpty: lipgloss.NewStyle().
			Background(base).
			Width(1).
			Render(" "),
		gaugeMuted: lipgloss.NewStyle().
			Background(base).
			Foreground(muted).
			Width(1).
			Render("│"),
		gaugeActive: lipgloss.NewStyle().
			Background(base).
			Foreground(highlight).
			Width(1).
			Render("█"),
		gaugeMark: lipgloss.NewStyle().
			Background(base).
			Foreground(text).
			Width(1).
			Render("■"),
		gaugeMetaEmpty: lipgloss.NewStyle().
			Background(base).
			Width(1).
			Render(" "),
		gaugeBoundaryMuted: lipgloss.NewStyle().
			Background(base).
			Foreground(muted).
			Width(1).
			Render("│"),
		gaugeBoundaryActive: lipgloss.NewStyle().
			Background(base).
			Foreground(highlight).
			Width(1).
			Render("┃"),
		gaugeDepthMuted: [4]string{
			lipgloss.NewStyle().Background(base).Foreground(muted).Width(1).Render("•"),
			lipgloss.NewStyle().Background(base).Foreground(muted).Width(1).Render("◦"),
			lipgloss.NewStyle().Background(base).Foreground(muted).Width(1).Render("▪"),
			lipgloss.NewStyle().Background(base).Foreground(muted).Width(1).Render("▫"),
		},
		gaugeDepthActive: [4]string{
			lipgloss.NewStyle().Background(base).Foreground(highlight).Width(1).Render("•"),
			lipgloss.NewStyle().Background(base).Foreground(highlight).Width(1).Render("◦"),
			lipgloss.NewStyle().Background(base).Foreground(highlight).Width(1).Render("▪"),
			lipgloss.NewStyle().Background(base).Foreground(highlight).Width(1).Render("▫"),
		},
		// Pre-computed SGR sequences
		sgrBgMain:        hexToBgSGR(p.bg),
		sgrBgSurface:     hexToBgSGR(p.surface),
		sgrBgHighlight:   hexToBgSGR(p.highlight),
		sgrBgWarn:        hexToBgSGR(p.warn),
		sgrFgText:        hexToFgSGR(p.text),
		sgrFgMuted:       hexToFgSGR(p.muted),
		sgrFgWarn:        hexToFgSGR(p.warn),
		sgrFgBorder:      hexToFgSGR(p.border),
		sgrFgBg:          hexToFgSGR(p.bg),
		sgrReset:         "\x1b[0m",
		sgrBold:          "\x1b[1m",
		sgrDimPrefix:     hexToFgSGR(p.muted) + hexToBgSGR(p.bg),
		sgrSearchPri:     hexToBgSGR(p.warn) + hexToFgSGR(p.bg) + "\x1b[1m",
		sgrSearchSec:     hexToBgSGR(p.highlight) + hexToFgSGR(p.text),
		sgrHighlight:     hexToBgSGR(p.highlight) + hexToFgSGR(p.text),
		sgrActiveBorder:  hexToFgSGR(p.warn) + hexToBgSGR(p.highlight),
		sgrBorderNormal:  hexToFgSGR(p.border) + hexToBgSGR(p.bg),
		codeBorderStyled: hexToFgSGR(p.warn) + hexToBgSGR(p.surface) + "▎ " + "\x1b[0m",
		lineNumFmt:       hexToFgSGR(p.muted) + hexToBgSGR(p.bg),
		lineNumBgWrap:    hexToBgSGR(p.bg),
		sgrReadingDim:    hexToFgSGR("#3a3a3a") + hexToBgSGR(p.bg),
		sgrFrameClear:    hexToBgSGR(p.bg) + "\x1b[K",
		sgrResetBg:       "\x1b[0m" + hexToBgSGR(p.bg),
	}
}

// hexToBgSGR converts a hex color like "#1e1f29" to an ANSI 24-bit background SGR sequence.
func hexToBgSGR(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return ""
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

// hexToFgSGR converts a hex color like "#f8f8f2" to an ANSI 24-bit foreground SGR sequence.
func hexToFgSGR(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return ""
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func paletteForStyle(name string) colorPalette {
	switch name {
	case styles.DraculaStyle:
		return colorPalette{
			bg:        "#1e1f29",
			surface:   "#282a36",
			border:    "#383a4a",
			text:      "#f8f8f2",
			muted:     "#bdc1c6",
			warn:      "#ff79c6",
			highlight: "#343746",
		}
	case styles.DarkStyle:
		return colorPalette{
			bg:        "#0b0f16",
			surface:   "#0f172a",
			border:    "#1f2937",
			text:      "#e2e8f0",
			muted:     "#94a3b8",
			warn:      "#fb7185",
			highlight: "#1b2337",
		}
	case styles.LightStyle:
		return colorPalette{
			bg:        "#f3f4f6",
			surface:   "#ffffff",
			border:    "#d1d5db",
			text:      "#111827",
			muted:     "#6b7280",
			warn:      "#b91c1c",
			highlight: "#e5e7eb",
		}
	case styles.TokyoNightStyle:
		return colorPalette{
			bg:        "#16161e",
			surface:   "#1a1b26",
			border:    "#2f334d",
			text:      "#c0caf5",
			muted:     "#9aa5ce",
			warn:      "#f7768e",
			highlight: "#24283b",
		}
	case styles.PinkStyle:
		return colorPalette{
			bg:        "#1f1024",
			surface:   "#27102f",
			border:    "#3a1f44",
			text:      "#f5e9f7",
			muted:     "#c8a9d4",
			warn:      "#ff7aa2",
			highlight: "#31123b",
		}
	case catppuccinStyle:
		return colorPalette{
			bg:        "#1e1e2e",
			surface:   "#313244",
			border:    "#45475a",
			text:      "#cdd6f4",
			muted:     "#a6adc8",
			warn:      "#f38ba8",
			highlight: "#2a2a3d",
		}
	case nordStyle:
		return colorPalette{
			bg:        "#2e3440",
			surface:   "#3b4252",
			border:    "#4c566a",
			text:      "#eceff4",
			muted:     "#d8dee9",
			warn:      "#bf616a",
			highlight: "#3b4252",
		}
	case gruvboxStyle:
		return colorPalette{
			bg:        "#1d2021",
			surface:   "#282828",
			border:    "#3c3836",
			text:      "#ebdbb2",
			muted:     "#a89984",
			warn:      "#fb4934",
			highlight: "#32302f",
		}
	case solarizedDarkStyle:
		return colorPalette{
			bg:        "#002b36",
			surface:   "#073642",
			border:    "#1b525d",
			text:      "#93a1a1",
			muted:     "#839496",
			warn:      "#dc322f",
			highlight: "#0f3a46",
		}
	case solarizedLightStyle:
		return colorPalette{
			bg:        "#fdf6e3",
			surface:   "#eee8d5",
			border:    "#d9cfb3",
			text:      "#586e75",
			muted:     "#657b83",
			warn:      "#cb4b16",
			highlight: "#e7dfc9",
		}
	case everforestStyle:
		return colorPalette{
			bg:        "#232a2e",
			surface:   "#2d353b",
			border:    "#475258",
			text:      "#d3c6aa",
			muted:     "#9da9a0",
			warn:      "#e67e80",
			highlight: "#343f44",
		}
	case kanagawaStyle:
		return colorPalette{
			bg:        "#1f1f28",
			surface:   "#2a2a37",
			border:    "#363646",
			text:      "#dcd7ba",
			muted:     "#938aa9",
			warn:      "#e46876",
			highlight: "#2d4f67",
		}
	default:
		return colorPalette{
			bg:        "#0f1117",
			surface:   "#111320",
			border:    "#2b2f3a",
			text:      "#e5e7eb",
			muted:     "#9ca3af",
			warn:      "#f87171",
			highlight: "#1f2937",
		}
	}
}

func systemPreferredStyle() string {
	val := strings.ToLower(os.Getenv("SYSTEM_THEME"))
	switch val {
	case "light":
		return styles.LightStyle
	case "dark":
		return defaultStyleName
	default:
		return ""
	}
}

func styleFromTerminalBackground(isDark bool) string {
	if isDark {
		return defaultStyleName
	}
	return styles.LightStyle
}

func (m model) preferredFollowStyle() string {
	if style := systemPreferredStyle(); style != "" {
		return style
	}
	if m.terminalBgKnown {
		return styleFromTerminalBackground(m.terminalBgDark)
	}
	return defaultStyleName
}

func preferredFollowSystem(initial string) bool { return systemPreferredStyle() != "" }

func sessionFilePath(dir string) string {
	return filepath.Join(dir, sessionFileName)
}

func modeLabel(mode viewMode) string {
	switch mode {
	case modeRaw:
		return "raw"
	case modeSplit:
		return "split"
	default:
		return "preview"
	}
}

func parseMode(label string) viewMode {
	switch {
	case strings.EqualFold(label, "raw"):
		return modeRaw
	case strings.EqualFold(label, "split"):
		return modeSplit
	default:
		return modePreview
	}
}

func perfVisualModeLabel(mode perfVisualMode) string {
	switch mode {
	case perfVisualForceOn:
		return "on"
	case perfVisualForceOff:
		return "off"
	default:
		return "auto"
	}
}

func parsePerfVisualMode(label string) perfVisualMode {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "on":
		return perfVisualForceOn
	case "off":
		return perfVisualForceOff
	default:
		return perfVisualAuto
	}
}

func envBool(key string) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func programFPSFromEnv() int {
	n := envInt("MARKDOWNVIEWER_FPS", defaultMaxFPS)
	if n < 1 {
		return defaultMaxFPS
	}
	if n > 120 {
		return 120
	}
	return n
}

func newPerfStateFromEnv() *perfState {
	enabled := envBool("MARKDOWNVIEWER_PERF")
	overlay := envBool("MARKDOWNVIEWER_PERF_OVERLAY")
	logPath := strings.TrimSpace(os.Getenv("MARKDOWNVIEWER_PERF_LOG"))
	if !enabled && !overlay && logPath == "" {
		return &perfState{}
	}
	p := &perfState{
		enabled:          true,
		overlay:          overlay,
		logPath:          logPath,
		frameWindowStart: time.Now(),
		logEvery:         20,
	}
	if rawEvery := strings.TrimSpace(os.Getenv("MARKDOWNVIEWER_PERF_LOG_EVERY")); rawEvery != "" {
		if n, err := strconv.Atoi(rawEvery); err == nil && n > 0 {
			p.logEvery = uint64(n)
		}
	}
	if logPath != "" {
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			p.logFile = f
		}
	}
	return p
}

func (p *perfState) setEnabled(v bool) {
	if p == nil {
		return
	}
	if p.enabled == v {
		return
	}
	p.enabled = v
	p.seq++
	if v && p.frameWindowStart.IsZero() {
		p.frameWindowStart = time.Now()
	}
}

func (p *perfState) setOverlay(v bool) {
	if p == nil {
		return
	}
	if p.overlay == v {
		return
	}
	p.overlay = v
	p.seq++
}

func ewma(prev float64, next float64) float64 {
	if prev <= 0 {
		return next
	}
	const alpha = 0.18
	return prev + alpha*(next-prev)
}

func (p *perfState) appendLog(event string, ms float64) {
	if p == nil || p.logFile == nil {
		return
	}
	if p.logEvery > 1 && p.seq%p.logEvery != 0 {
		return
	}
	_, _ = fmt.Fprintf(
		p.logFile,
		"{\"ts\":\"%s\",\"event\":\"%s\",\"ms\":%.3f,\"fps\":%.2f,\"view_avg_ms\":%.3f,\"layout_avg_ms\":%.3f,\"render_avg_ms\":%.3f}\n",
		time.Now().Format(time.RFC3339Nano),
		event,
		ms,
		p.fps,
		p.viewAvgMs,
		p.layoutAvgMs,
		p.renderAvgMs,
	)
}

func (p *perfState) recordDuration(event string, d time.Duration) {
	if p == nil || !p.enabled {
		return
	}
	ms := float64(d) / float64(time.Millisecond)
	switch event {
	case "view":
		p.viewLastMs = ms
		p.viewAvgMs = ewma(p.viewAvgMs, ms)
		now := time.Now()
		if p.frameWindowStart.IsZero() {
			p.frameWindowStart = now
		}
		p.frameCount++
		window := now.Sub(p.frameWindowStart)
		if window >= time.Second {
			p.fps = float64(p.frameCount) / window.Seconds()
			p.frameWindowStart = now
			p.frameCount = 0
		}
	case "layout":
		p.layoutLastMs = ms
		p.layoutAvgMs = ewma(p.layoutAvgMs, ms)
	case "render":
		p.renderLastMs = ms
		p.renderAvgMs = ewma(p.renderAvgMs, ms)
	}
	p.seq++
	p.appendLog(event, ms)
}

func (m *model) togglePerfOverlay() {
	if m.perf == nil {
		m.perf = &perfState{}
	}
	m.perf.setEnabled(true)
	m.perf.setOverlay(!m.perf.overlay)
	if m.perf.overlay {
		m.status = "Runtime perf overlay on"
	} else {
		m.status = "Runtime perf overlay off"
	}
}

func (m model) perfStatusSummary() string {
	if m.perf == nil || !m.perf.enabled || !m.perf.overlay {
		return ""
	}
	return fmt.Sprintf(
		"PERF %.0ffps v%.2f/%.2f l%.2f/%.2f r%.2f/%.2f",
		m.perf.fps,
		m.perf.viewLastMs, m.perf.viewAvgMs,
		m.perf.layoutLastMs, m.perf.layoutAvgMs,
		m.perf.renderLastMs, m.perf.renderAvgMs,
	)
}

func readSession(path string) (sessionState, error) {
	var state sessionState
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func writeSessionFile(path string, state sessionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (m *model) applySession(state sessionState) {
	m.currentPath = state.CurrentPath
	m.mode = parseMode(state.Mode)
	if state.ListWidthRatio > 0 {
		m.listWidthRatio = state.ListWidthRatio
	}
	m.showOutline = state.ShowOutline
	m.fullScreen = state.FullScreen
	m.focusRight = state.FocusRight
	m.richPreview = state.RichPreview
	if state.Zoom > 0 {
		m.zoom = state.Zoom
	}
	m.showLineNums = state.ShowLineNums
	m.textarea.ShowLineNumbers = m.showLineNums
	m.focusMode = state.FocusMode
	m.readingMode = state.ReadingMode
	m.codePlain = state.CodePlain
	m.reducedMotion = state.ReducedMotion
	m.formatOnSave = state.FormatOnSave
	m.editHomeEndWrapped = state.EditHomeEndWrapped
	m.showGauge = state.ShowGauge
	m.perfVisualMode = parsePerfVisualMode(state.PerfVisualMode)
	m.lastPerfAutoDisable = m.autoDisableHeavyVisuals()
	if state.PreviewYOffset >= 0 {
		m.previewYOffset = state.PreviewYOffset
	}
	m.restoreHeading = state.CurrentHeading
	m.restoreSession = true
	m.followSystem = state.FollowSystem
	m.showStats = state.ShowStats
	m.autoSave = state.AutoSave
	m.editSoftWrap = state.EditSoftWrap
	// Restore bookmarks
	if len(state.Bookmarks) > 0 {
		m.bookmarks = make(map[rune]bookmarkEntry, len(state.Bookmarks))
		for k, v := range state.Bookmarks {
			runes := []rune(k)
			if len(runes) == 1 {
				m.bookmarks[runes[0]] = v
			}
		}
	}
	// Restore folded sections
	if len(state.FoldedSections) > 0 {
		m.foldedSections = make(map[int]bool, len(state.FoldedSections))
		for _, idx := range state.FoldedSections {
			m.foldedSections[idx] = true
		}
	}
	if state.FollowSystem {
		style := m.preferredFollowStyle()
		_ = m.applyTheme(style)
	} else if state.StyleName != "" {
		_ = m.applyTheme(state.StyleName)
	}
}

func (m model) buildSessionState() sessionState {
	yOffset := m.previewYOffset
	if m.mode == modePreview {
		yOffset = m.viewport.YOffset()
	}
	return sessionState{
		Version:            sessionVersion,
		CurrentPath:        m.currentPath,
		Mode:               modeLabel(m.mode),
		PreviewYOffset:     yOffset,
		ListWidthRatio:     m.listWidthRatio,
		ShowOutline:        m.showOutline,
		FullScreen:         m.fullScreen,
		FocusRight:         m.focusRight,
		RichPreview:        m.richPreview,
		StyleName:          m.styleName,
		FollowSystem:       m.followSystem,
		Zoom:               m.zoom,
		ShowLineNums:       m.showLineNums,
		FocusMode:          m.focusMode,
		ReadingMode:        m.readingMode,
		CodePlain:          m.codePlain,
		ReducedMotion:      m.reducedMotion,
		FormatOnSave:       m.formatOnSave,
		EditHomeEndWrapped: m.editHomeEndWrapped,
		ShowGauge:          m.showGauge,
		PerfVisualMode:     perfVisualModeLabel(m.perfVisualMode),
		CurrentHeading:     m.currentHeading,
		ShowStats:          m.showStats,
		AutoSave:           m.autoSave,
		EditSoftWrap:       m.editSoftWrap,
		Bookmarks:          m.buildBookmarksState(),
		FoldedSections:     m.buildFoldedSectionsState(),
	}
}

func (m model) buildBookmarksState() map[string]bookmarkEntry {
	if len(m.bookmarks) == 0 {
		return nil
	}
	out := make(map[string]bookmarkEntry, len(m.bookmarks))
	for k, v := range m.bookmarks {
		out[string(k)] = v
	}
	return out
}

func (m model) buildFoldedSectionsState() []int {
	if len(m.foldedSections) == 0 {
		return nil
	}
	out := make([]int, 0, len(m.foldedSections))
	for idx := range m.foldedSections {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func (m model) saveAndQuitCmd() tea.Cmd {
	if m.sessionPath == "" {
		return tea.Quit
	}
	state := m.buildSessionState()
	path := m.sessionPath
	return func() tea.Msg {
		_ = writeSessionFile(path, state)
		return tea.Quit()
	}
}

func newSearchInput(palette colorPalette) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Search…"
	ti.CharLimit = 256
	ti.SetWidth(30)
	ti.Prompt = "/ "
	applySearchInputTheme(&ti, palette)
	return ti
}

func newReplaceInput(palette colorPalette) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Replace…"
	ti.CharLimit = 256
	ti.SetWidth(30)
	ti.Prompt = "r "
	applySearchInputTheme(&ti, palette)
	return ti
}

func newPromptInput(palette colorPalette) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "filename.md"
	ti.CharLimit = 256
	ti.SetWidth(40)
	ti.Prompt = "> "
	applySearchInputTheme(&ti, palette)
	return ti
}

func applySearchInputTheme(ti *textinput.Model, palette colorPalette) {
	bg := lipgloss.Color(palette.bg)
	text := lipgloss.Color(palette.text)
	muted := lipgloss.Color(palette.muted)
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Background(bg).Foreground(text)
	s.Focused.Text = lipgloss.NewStyle().Background(bg).Foreground(text)
	s.Focused.Placeholder = lipgloss.NewStyle().Background(bg).Foreground(muted)
	s.Blurred = s.Focused
	ti.SetStyles(s)
}

func applyTextareaTheme(ta *textarea.Model, palette colorPalette) tea.Cmd {
	base := lipgloss.NewStyle().
		Background(lipgloss.Color(palette.bg)).
		Foreground(lipgloss.Color(palette.text))
	muted := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.muted)).
		Background(lipgloss.Color(palette.bg))
	cursorLine := base.Background(lipgloss.Color(palette.highlight))

	focused := textarea.StyleState{
		Base:             base,
		CursorLine:       cursorLine,
		CursorLineNumber: muted,
		EndOfBuffer:      muted,
		LineNumber:       muted,
		Placeholder:      muted,
		Prompt:           base,
		Text:             base,
	}
	blurred := focused
	blurred.CursorLine = base
	blurred.Text = base.Foreground(lipgloss.Color(palette.muted))

	s := ta.Styles()
	s.Focused = focused
	s.Blurred = blurred
	ta.SetStyles(s)
	if ta.Focused() {
		return ta.Focus()
	}
	ta.Blur()
	return nil
}

func applyListTheme(l *list.Model, d *list.DefaultDelegate, palette colorPalette, accent bool) {
	styles := list.DefaultStyles(true)
	baseBg := lipgloss.Color(palette.bg)
	text := lipgloss.Color(palette.text)
	muted := lipgloss.Color(palette.muted)
	highlight := lipgloss.Color(palette.highlight)
	accentColor := lipgloss.Color(palette.border)

	styles.TitleBar = styles.TitleBar.Background(baseBg).Foreground(text).Padding(0)
	styles.Title = styles.Title.Background(baseBg).Foreground(text).Padding(0)
	styles.StatusBar = styles.StatusBar.Background(baseBg).Foreground(muted).Padding(0)
	styles.StatusEmpty = styles.StatusEmpty.Background(baseBg).Foreground(muted)
	styles.StatusBarActiveFilter = styles.StatusBarActiveFilter.Background(baseBg).Foreground(text)
	styles.StatusBarFilterCount = styles.StatusBarFilterCount.Background(baseBg).Foreground(muted)
	styles.NoItems = styles.NoItems.Background(baseBg).Foreground(muted)
	styles.PaginationStyle = styles.PaginationStyle.Background(baseBg).Foreground(muted).Padding(0)
	styles.HelpStyle = styles.HelpStyle.Background(baseBg).Foreground(muted).Padding(0)
	styles.ArabicPagination = styles.ArabicPagination.Background(baseBg).Foreground(muted)
	styles.ActivePaginationDot = styles.ActivePaginationDot.Background(baseBg)
	styles.InactivePaginationDot = styles.InactivePaginationDot.Background(baseBg)
	styles.DividerDot = styles.DividerDot.Background(baseBg)
	l.Styles = styles

	if d != nil {
		s := d.Styles
		s.NormalTitle = s.NormalTitle.Background(baseBg).Foreground(text).Padding(0)
		s.NormalDesc = s.NormalDesc.Background(baseBg).Foreground(muted).Padding(0)
		s.SelectedTitle = s.SelectedTitle.Background(highlight).Foreground(text).BorderStyle(lipgloss.Border{}).Padding(0)
		s.SelectedDesc = s.SelectedDesc.Background(highlight).Foreground(text).BorderStyle(lipgloss.Border{}).Padding(0)
		s.DimmedTitle = s.DimmedTitle.Background(baseBg).Foreground(muted).Padding(0)
		s.DimmedDesc = s.DimmedDesc.Background(baseBg).Foreground(muted).Padding(0)
		s.FilterMatch = s.FilterMatch.Background(baseBg).Foreground(text)
		if accent {
			s.NormalTitle = s.NormalTitle.Padding(0, 0, 0, 1)
			s.DimmedTitle = s.DimmedTitle.Padding(0, 0, 0, 1)
			s.SelectedTitle = s.SelectedTitle.
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(accentColor).
				Padding(0)
		}
		d.Styles = s
		l.SetDelegate(*d)
	}
}

func (m *model) applyTheme(name string) tea.Cmd {
	m.palette = paletteForStyle(name)
	m.styles = buildStyles(m.palette)
	m.invalidateRenderCaches()
	m.styleName = name
	m.themePickerIdx = themeIndex(name)
	applyListTheme(&m.fileList, &m.fileDelegate, m.palette, true)
	applyListTheme(&m.outline, &m.outDelegate, m.palette, false)
	applySearchInputTheme(&m.searchInput, m.palette)
	applySearchInputTheme(&m.replaceInput, m.palette)
	applySearchInputTheme(&m.promptInput, m.palette)
	return applyTextareaTheme(&m.textarea, m.palette)
}

func newModel() model {
	return newModelWithConfig(defaultAppConfig())
}

func newModelWithConfig(cfg appConfig) model {
	if cfg.dir == "" {
		cfg = defaultAppConfig()
	}
	sessionPath := cfg.sessionPath
	perf := newPerfStateFromEnv()
	sessionLoaded := false
	var session sessionState
	if sessionPath != "" {
		if loaded, err := readSession(sessionPath); err == nil && (loaded.Version == 0 || loaded.Version == sessionVersion) {
			session = loaded
			sessionLoaded = true
		}
	}
	initialStyle := defaultStyleName
	if preferred := systemPreferredStyle(); preferred != "" {
		initialStyle = preferred
	}
	palette := paletteForStyle(initialStyle)
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	l := list.New([]list.Item{}, delegate, 30, 10)
	l.Title = ""
	l.SetShowTitle(false)
	l.KeyMap.CursorUp = key.NewBinding(key.WithKeys("up"))
	l.KeyMap.CursorDown = key.NewBinding(key.WithKeys("down"))
	l.KeyMap.PrevPage = key.NewBinding(key.WithKeys("pgup"))
	l.KeyMap.NextPage = key.NewBinding(key.WithKeys("pgdown"))
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)

	outDelegate := list.NewDefaultDelegate()
	outDelegate.ShowDescription = true
	ol := list.New([]list.Item{}, outDelegate, 30, 10)
	ol.Title = ""
	ol.SetShowTitle(false)
	ol.KeyMap.CursorUp = key.NewBinding(key.WithKeys("up"))
	ol.KeyMap.CursorDown = key.NewBinding(key.WithKeys("down"))
	ol.KeyMap.PrevPage = key.NewBinding(key.WithKeys("pgup"))
	ol.KeyMap.NextPage = key.NewBinding(key.WithKeys("pgdown"))
	ol.SetShowStatusBar(false)
	ol.SetFilteringEnabled(false)
	ol.SetShowHelp(false)
	ol.SetShowPagination(false)

	ta := textarea.New()
	ta.Placeholder = "No file selected"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetWidth(0)
	ta.SetHeight(0)
	applyTextareaTheme(&ta, palette)
	applyListTheme(&l, &delegate, palette, true)
	applyListTheme(&ol, &outDelegate, palette, false)

	vp := viewport.New()
	vp.MouseWheelEnabled = false                    // we handle wheel events with fractional accumulation
	vp.KeyMap.Down = key.NewBinding(key.WithKeys()) // handled by momentum scroll
	vp.KeyMap.Up = key.NewBinding(key.WithKeys())   // handled by momentum scroll
	vp.KeyMap.HalfPageDown = key.NewBinding(key.WithKeys("ctrl+d"))
	vp.KeyMap.HalfPageUp = key.NewBinding(key.WithKeys("ctrl+u"))
	vp.KeyMap.PageDown = key.NewBinding(key.WithKeys("pgdown"))
	vp.KeyMap.PageUp = key.NewBinding(key.WithKeys("pgup"))
	search := newSearchInput(palette)
	replace := newReplaceInput(palette)
	prompt := newPromptInput(palette)

	var watcher *fsnotify.Watcher
	if w, err := fsnotify.NewWatcher(); err == nil {
		if err := w.Add(cfg.dir); err == nil {
			watcher = w
		} else {
			_ = w.Close()
		}
	}

	m := model{
		dir:                 cfg.dir,
		fileList:            l,
		fileTreeExpanded:    make(map[string]bool),
		outline:             ol,
		viewport:            vp,
		textarea:            ta,
		mode:                modePreview,
		status:              "Loading files…",
		richPreview:         true,
		styleName:           initialStyle,
		styles:              buildStyles(palette),
		palette:             palette,
		fileDelegate:        delegate,
		outDelegate:         outDelegate,
		listWidthRatio:      0.33,
		themePickerIdx:      themeIndex(initialStyle),
		followSystem:        preferredFollowSystem(initialStyle),
		searchInput:         search,
		replaceInput:        replace,
		promptInput:         prompt,
		watcher:             watcher,
		zoom:                1.0,
		showLineNums:        false,
		focusRight:          false,
		terminalFocused:     true,
		fullScreen:          false,
		reducedMotion:       false,
		showGauge:           true,
		perfVisualMode:      perfVisualAuto,
		focusMode:           false,
		codePlain:           false,
		showHelp:            false,
		highlightLine:       -1,
		editFocusLine:       -1,
		currentHeading:      -1,
		restoreHeading:      -1,
		sessionPath:         sessionPath,
		layoutCache:         &layoutCache{breadcrumbHdg: -1},
		mermaid:             newMermaidSupport(),
		perf:                perf,
		editSelAnchorOffset: -1,
		editMouseAnchorOff:  -1,
		editSoftWrap:        true,
		bookmarks:           make(map[rune]bookmarkEntry),
		foldedSections:      make(map[int]bool),
		fileFinderInput:     newFileFinderInput(palette),
		contentSearchInput:  newContentSearchInput(palette),
	}
	if sessionLoaded {
		m.applySession(session)
	}
	if cfg.initialPath != "" {
		m.currentPath = cfg.initialPath
		m.previewYOffset = 0
		m.restoreHeading = -1
		m.restoreSession = false
	}
	m.updateBreadcrumb()
	return m
}

func newFileFinderInput(palette colorPalette) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.CharLimit = 256
	ti.SetWidth(60)
	return ti
}

func newContentSearchInput(palette colorPalette) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Search across files..."
	ti.CharLimit = 256
	ti.SetWidth(60)
	return ti
}

func collapsePasteToLine(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	parts := strings.Split(content, "\n")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, " ")
}

func firstPasteLine(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if i := strings.IndexByte(content, '\n'); i >= 0 {
		content = content[:i]
	}
	return strings.TrimSpace(content)
}

func (m *model) openSearch() tea.Cmd {
	m.showSearch = true
	m.showReplace = false
	m.replaceInputFocus = false
	query := ""
	if m.mode == modeRaw && m.editHasSelection() {
		query = collapsePasteToLine(m.editSelectedText())
	}
	m.searchInput.SetValue(query)
	m.searchIndex = -1
	m.searchMatches = nil
	m.searchReturnSet = false
	if strings.TrimSpace(query) != "" {
		m.refreshSearchMatches()
	}
	if m.mode == modeRaw {
		m.searchReturnRow = m.textarea.Line()
		m.searchReturnCol = m.editCursorCol()
		m.searchReturnSet = true
	}
	cmd := m.searchInput.Focus()
	m.replaceInput.Blur()
	m.status = "Search"
	m.resizeViews()
	return cmd
}

func (m *model) setReplaceInputFocus(focusReplace bool) tea.Cmd {
	m.replaceInputFocus = focusReplace
	if focusReplace {
		m.searchInput.Blur()
		return m.replaceInput.Focus()
	}
	m.replaceInput.Blur()
	return m.searchInput.Focus()
}

func (m *model) openReplace() tea.Cmd {
	cmds := []tea.Cmd{}
	if !m.showSearch {
		cmds = append(cmds, m.openSearch())
	}
	m.showSearch = true
	m.showReplace = true
	if m.mode == modeRaw && m.editHasSelection() {
		m.replaceScopeSelection = true
	} else {
		m.replaceScopeSelection = false
	}
	m.searchIndex = -1
	m.refreshSearchMatches()
	cmds = append(cmds, m.setReplaceInputFocus(true))
	m.status = "Replace"
	m.resizeViews()
	return tea.Batch(cmds...)
}

func (m *model) closeReplace() {
	if !m.showReplace {
		return
	}
	m.showReplace = false
	m.replaceInputFocus = false
	m.replaceInput.Blur()
	_ = m.searchInput.Focus()
	m.searchIndex = -1
	m.refreshSearchMatches()
	m.status = "Replace closed"
	m.resizeViews()
}

func (m *model) closeSearch(restoreRawCursor bool) {
	if restoreRawCursor && m.mode == modeRaw && m.searchReturnSet {
		m.editClearSelection()
		m.editSetCursor(m.searchReturnRow, m.searchReturnCol)
		m.editClearPreferredColumn()
	}
	m.showSearch = false
	m.showReplace = false
	m.replaceInputFocus = false
	m.searchMatches = nil
	m.searchInput.Blur()
	m.replaceInput.Blur()
	m.searchReturnSet = false
	m.status = "Search closed"
	m.resizeViews()
}

func (m *model) editRawContentRect() (x, y, w, h int, ok bool) {
	if m.mode != modeRaw || m.width <= 0 || m.height <= 0 {
		return 0, 0, 0, 0, false
	}
	bodyTop := m.breadcrumbRows() + m.promptRows()
	bodyHeight := m.availableContentHeight()
	minBodyHeight := 3
	if m.fullScreen {
		minBodyHeight = 1
	}
	if bodyHeight < minBodyHeight {
		return 0, 0, 0, 0, false
	}
	paneX := 0
	if !m.fullScreen {
		paneX = m.listWidth() + 1
	}
	paneY := bodyTop
	if m.fullScreen {
		x = paneX
		y = paneY
		w = max(1, m.rightWidth()-1)
		h = max(1, bodyHeight)
	} else {
		x = paneX + 1
		y = paneY + 1
		w = max(1, m.rightWidth()-2)
		h = max(1, bodyHeight-2)
	}
	return x, y, w, h, true
}

func (m model) previewPaneRect() (x, y, w, h int, ok bool) {
	if m.mode != modePreview || m.width <= 0 || m.height <= 0 {
		return 0, 0, 0, 0, false
	}
	bodyTop := m.breadcrumbRows() + m.promptRows()
	bodyHeight := m.availableContentHeight()
	minBodyHeight := 3
	if m.fullScreen {
		minBodyHeight = 1
	}
	if bodyHeight < minBodyHeight {
		return 0, 0, 0, 0, false
	}
	paneX := 0
	if !m.fullScreen {
		paneX = m.listWidth() + 1
	}
	paneY := bodyTop
	if m.fullScreen {
		return paneX, paneY, max(1, m.rightWidth()), max(1, bodyHeight), true
	}
	return paneX, paneY, max(1, m.rightWidth()-2), max(1, bodyHeight-2), true
}

func scrollbarTargetOffset(row, grabOffset, height, totalRows, visibleRows, maxOffset int, insetEnds bool) int {
	if maxOffset <= 0 || height <= 1 {
		return 0
	}
	geometry := calculateScrollbarGeometry(height, totalRows, visibleRows, 0, insetEnds)
	thumbLen := geometry.thumbEnd - geometry.thumbStart
	travel := geometry.trackEnd - geometry.trackStart - thumbLen
	if travel <= 0 {
		return 0
	}
	thumbStart := row - grabOffset
	thumbStart = max(geometry.trackStart, min(thumbStart, geometry.trackStart+travel))
	pct := float64(thumbStart-geometry.trackStart) / float64(travel)
	return int(math.Round(pct * float64(maxOffset)))
}

func scrollbarGrabOffset(row int, geometry scrollbarGeometry) int {
	thumbLen := geometry.thumbEnd - geometry.thumbStart
	if thumbLen <= 0 {
		return 0
	}
	if row >= geometry.thumbStart && row < geometry.thumbEnd {
		return row - geometry.thumbStart
	}
	return thumbLen / 2
}

func (m model) previewScrollbarRowFromMouse(x, y int, clampToBounds bool) (row int, ok bool) {
	paneX, paneY, _, innerH, ok := m.previewPaneRect()
	if !ok {
		return 0, false
	}
	contentX := paneX
	contentY := paneY
	if !m.fullScreen {
		contentX++
		contentY++
	}
	scrollbarX := contentX + m.viewport.Width()
	if !clampToBounds {
		if x != scrollbarX || y < contentY || y >= contentY+innerH {
			return 0, false
		}
	} else {
		y = max(contentY, min(y, contentY+innerH-1))
	}
	return y - contentY, true
}

func (m *model) setPreviewOffsetFromScrollbarRow(row int) {
	maxOffset := max(0, m.renderedLineCount-m.viewport.Height())
	target := scrollbarTargetOffset(
		row,
		m.scrollbarDragGrab,
		m.viewport.Height(),
		m.renderedLineCount,
		m.viewport.Height(),
		maxOffset,
		!m.fullScreen,
	)
	m.cancelMomentum()
	m.focusRight = true
	m.viewport.SetYOffset(target)
	m.previewYOffset = m.viewport.YOffset()
	m.syncCurrentHeading(m.previewYOffset)
}

func (m model) rawScrollbarRowFromMouse(x, y int, clampToBounds bool) (row int, ok bool) {
	contentX, contentY, _, contentH, ok := m.editRawContentRect()
	if !ok {
		return 0, false
	}
	scrollbarX := contentX + m.textarea.Width()
	if !clampToBounds {
		if x != scrollbarX || y < contentY || y >= contentY+contentH {
			return 0, false
		}
	} else {
		y = max(contentY, min(y, contentY+contentH-1))
	}
	return y - contentY, true
}

func (m model) gaugeRowFromMouse(x, y int) (row int, ok bool) {
	if !m.showSectionGauge() {
		return 0, false
	}
	paneX, paneY, _, innerH, ok := m.previewPaneRect()
	if !ok {
		return 0, false
	}
	contentX := paneX
	contentY := paneY
	if !m.fullScreen {
		contentX++
		contentY++
	}
	if y < contentY || y >= contentY+innerH {
		return 0, false
	}
	gaugeStartX := contentX + m.viewport.Width() + 2 // content + scrollbar + separator
	gaugeEndX := gaugeStartX + m.sectionGaugeWidth()
	if x < gaugeStartX || x >= gaugeEndX {
		return 0, false
	}
	return y - contentY, true
}

func (m model) gaugeTargetForRow(row int) (line int, headingIdx int, ok bool) {
	height := m.viewport.Height()
	total := m.renderedLineCount
	if height <= 0 || total <= 0 {
		return 0, -1, false
	}
	targetLine := lineForGaugeRow(row, total, height)
	bestIdx := -1
	bestDist := int(^uint(0) >> 1)
	for i, h := range m.headings {
		if h.renderLine < 0 {
			continue
		}
		if rowForRenderLine(h.renderLine, total, height) != row {
			continue
		}
		dist := abs(h.renderLine - targetLine)
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return m.headings[bestIdx].renderLine, bestIdx, true
	}
	return targetLine, -1, true
}

func (m *model) handlePreviewGaugeMouseClick(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft || m.mode != modePreview {
		return false
	}
	row, ok := m.gaugeRowFromMouse(msg.X, msg.Y)
	if !ok {
		return false
	}
	targetLine, headingIdx, ok := m.gaugeTargetForRow(row)
	if !ok {
		return false
	}
	m.focusRight = true
	_ = m.scrollTo(targetLine)
	if headingIdx >= 0 {
		m.setCurrentHeading(headingIdx)
		m.highlightLine = m.headings[headingIdx].renderLine
		m.updateBreadcrumb()
		m.status = fmt.Sprintf("Jumped to %s", m.headings[headingIdx].title)
	} else {
		m.status = fmt.Sprintf("Jumped to line %d", targetLine+1)
	}
	return true
}

func (m *model) handlePreviewScrollbarMouseClick(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft || m.mode != modePreview {
		return false
	}
	if m.renderedLineCount <= m.viewport.Height() {
		return false
	}
	row, ok := m.previewScrollbarRowFromMouse(msg.X, msg.Y, false)
	if !ok {
		return false
	}
	geometry := calculateScrollbarGeometry(
		m.viewport.Height(),
		m.renderedLineCount,
		m.viewport.Height(),
		m.viewport.ScrollPercent(),
		!m.fullScreen,
	)
	m.scrollbarDragGrab = scrollbarGrabOffset(row, geometry)
	m.setPreviewOffsetFromScrollbarRow(row)
	m.scrollbarDrag = scrollbarDragPreview
	return true
}

func (m *model) handlePreviewScrollbarMouseDrag(msg tea.MouseMotionMsg) bool {
	if m.scrollbarDrag != scrollbarDragPreview || m.mode != modePreview {
		return false
	}
	if msg.Button != tea.MouseLeft && msg.Button != tea.MouseNone {
		return false
	}
	row, ok := m.previewScrollbarRowFromMouse(msg.X, msg.Y, true)
	if !ok {
		return false
	}
	m.setPreviewOffsetFromScrollbarRow(row)
	return true
}

func (m *model) handlePreviewScrollbarMouseRelease(msg tea.MouseReleaseMsg) bool {
	if m.scrollbarDrag != scrollbarDragPreview {
		return false
	}
	m.scrollbarDrag = scrollbarDragNone
	m.scrollbarDragGrab = 0
	return true
}

func editDisplayToSourcePoint(lines []string, width, displayRow, displayCol int) (row, col int, ok bool) {
	if len(lines) == 0 {
		return 0, 0, false
	}
	if width <= 0 {
		width = 1
	}
	if displayRow < 0 {
		displayRow = 0
	}
	if displayCol < 0 {
		displayCol = 0
	}
	remain := displayRow
	for rowIdx, line := range lines {
		lineRunes := []rune(line)
		segments := editWrapLine(lineRunes, width)
		if remain < len(segments) {
			segmentStart := 0
			for i := range remain {
				segmentStart += len(segments[i])
			}
			segmentLen := len(segments[remain])
			col = segmentStart + min(displayCol, segmentLen)
			if col > len(lineRunes) {
				col = len(lineRunes)
			}
			return rowIdx, col, true
		}
		remain -= len(segments)
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last])), true
}

func (m *model) rawEditorMouseToOffset(x, y int, clampToBounds bool) (offset, row, col int, ok bool) {
	contentX, contentY, contentW, contentH, ok := m.editRawContentRect()
	if !ok {
		return 0, 0, 0, false
	}
	if !clampToBounds && (x < contentX || x >= contentX+contentW || y < contentY || y >= contentY+contentH) {
		return 0, 0, 0, false
	}
	if clampToBounds {
		x = max(contentX, min(x, contentX+contentW-1))
		y = max(contentY, min(y, contentY+contentH-1))
	}
	localX := x - contentX
	localY := y - contentY
	textWidth := max(1, m.textarea.Width())
	if localX >= textWidth {
		localX = textWidth - 1
	}
	prefixW := m.editRenderedPrefixWidth()
	displayCol := localX - prefixW
	if displayCol < 0 {
		displayCol = 0
	}
	maxDisplayCol := max(0, textWidth-prefixW)
	if displayCol > maxDisplayCol {
		displayCol = maxDisplayCol
	}
	displayRow := m.textarea.ScrollYOffset() + localY
	lines := strings.Split(m.textarea.Value(), "\n")
	row, col, ok = editDisplayToSourcePoint(lines, textWidth, displayRow, displayCol)
	if !ok {
		return 0, 0, 0, false
	}
	offset = editRowColToRuneOffset(lines, row, col)
	return offset, row, col, true
}

func (m *model) rawEditorCellForSource(row, col int) (x, y int, ok bool) {
	contentX, contentY, _, contentH, ok := m.editRawContentRect()
	if !ok {
		return 0, 0, false
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) == 0 {
		return 0, 0, false
	}
	displayRow, displayCol := editDisplayPoint(lines, max(1, m.textarea.Width()), row, col)
	visibleRow := displayRow - m.textarea.ScrollYOffset()
	if visibleRow < 0 || visibleRow >= contentH {
		return 0, 0, false
	}
	x = contentX + m.editRenderedPrefixWidth() + displayCol
	y = contentY + visibleRow
	return x, y, true
}

func editWordBoundsOnLine(lineRunes []rune, col int) (start, end int) {
	if len(lineRunes) == 0 {
		return 0, 0
	}
	if col < 0 {
		col = 0
	}
	if col >= len(lineRunes) {
		col = len(lineRunes) - 1
	}
	class := editRuneClass(lineRunes[col])
	start = col
	for start > 0 && editRuneClass(lineRunes[start-1]) == class {
		start--
	}
	end = col + 1
	for end < len(lineRunes) && editRuneClass(lineRunes[end]) == class {
		end++
	}
	return start, end
}

func (m *model) handleRawEditorMouseClick(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft || m.mode != modeRaw {
		return false
	}
	offset, row, col, ok := m.rawEditorMouseToOffset(msg.X, msg.Y, false)
	if !ok {
		return false
	}
	m.focusRight = true
	_ = m.textarea.Focus()
	m.cancelMomentum()
	m.editResetUndoCoalescing()
	m.editClearPreferredColumn()
	now := time.Now()
	deltaRow := row - m.editLastClickRow
	if deltaRow < 0 {
		deltaRow = -deltaRow
	}
	deltaCol := col - m.editLastClickCol
	if deltaCol < 0 {
		deltaCol = -deltaCol
	}
	if now.Sub(m.editLastClickAt) <= editMultiClickGap &&
		deltaRow == 0 &&
		deltaCol <= editMultiClickTolerance {
		if m.editClickCount < 3 {
			m.editClickCount++
		}
	} else {
		m.editClickCount = 1
	}
	m.editLastClickAt = now
	m.editLastClickRow = row
	m.editLastClickCol = col

	lines := strings.Split(m.textarea.Value(), "\n")
	switch m.editClickCount {
	case 1:
		m.editMouseSelecting = true
		m.editMouseAnchorOff = offset
		m.editSetSelectionOffsets(offset, offset)
	case 2:
		if row < 0 || row >= len(lines) {
			return true
		}
		lineRunes := []rune(lines[row])
		startCol, endCol := editWordBoundsOnLine(lineRunes, col)
		start := editRowColToRuneOffset(lines, row, startCol)
		end := editRowColToRuneOffset(lines, row, endCol)
		m.editSetSelectionOffsets(start, end)
		m.editMouseSelecting = false
		m.editMouseAnchorOff = -1
	default:
		if row < 0 || row >= len(lines) {
			return true
		}
		start := editRowColToRuneOffset(lines, row, 0)
		end := editRowColToRuneOffset(lines, row, len([]rune(lines[row])))
		m.editSetSelectionOffsets(start, end)
		m.editMouseSelecting = false
		m.editMouseAnchorOff = -1
	}
	return true
}

func (m *model) handleRawScrollbarMouseClick(msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft || m.mode != modeRaw {
		return false
	}
	height, totalRows, maxOffset := m.editScrollMetrics()
	if maxOffset <= 0 {
		return false
	}
	row, ok := m.rawScrollbarRowFromMouse(msg.X, msg.Y, false)
	if !ok {
		return false
	}
	geometry := calculateScrollbarGeometry(
		height,
		totalRows,
		height,
		scrollbarPercent(m.textarea.ScrollYOffset(), maxOffset),
		!m.fullScreen,
	)
	m.scrollbarDragGrab = scrollbarGrabOffset(row, geometry)
	m.setRawOffsetFromScrollbarRow(row)
	m.scrollbarDrag = scrollbarDragRaw
	return true
}

func (m *model) handleRawScrollbarMouseDrag(msg tea.MouseMotionMsg) bool {
	if m.scrollbarDrag != scrollbarDragRaw || m.mode != modeRaw {
		return false
	}
	if msg.Button != tea.MouseLeft && msg.Button != tea.MouseNone {
		return false
	}
	row, ok := m.rawScrollbarRowFromMouse(msg.X, msg.Y, true)
	if !ok {
		return false
	}
	m.setRawOffsetFromScrollbarRow(row)
	return true
}

func (m *model) handleRawEditorMouseDrag(msg tea.MouseMotionMsg) bool {
	if !m.editMouseSelecting || m.mode != modeRaw {
		return false
	}
	if msg.Button != tea.MouseLeft && msg.Button != tea.MouseNone {
		return false
	}
	offset, _, _, ok := m.rawEditorMouseToOffset(msg.X, msg.Y, true)
	if !ok {
		return false
	}
	if m.editMouseAnchorOff < 0 {
		m.editMouseAnchorOff = m.editCursorOffset()
	}
	m.editSetSelectionOffsets(m.editMouseAnchorOff, offset)
	return true
}

func (m *model) handleRawEditorMouseRelease(msg tea.MouseReleaseMsg) bool {
	if msg.Button != tea.MouseLeft || m.mode != modeRaw {
		return false
	}
	if !m.editMouseSelecting {
		_, _, _, ok := m.rawEditorMouseToOffset(msg.X, msg.Y, false)
		return ok
	}
	offset, _, _, ok := m.rawEditorMouseToOffset(msg.X, msg.Y, true)
	if ok {
		if m.editMouseAnchorOff < 0 {
			m.editMouseAnchorOff = m.editCursorOffset()
		}
		m.editSetSelectionOffsets(m.editMouseAnchorOff, offset)
	}
	m.editMouseSelecting = false
	m.editMouseAnchorOff = -1
	if !m.editHasSelection() {
		m.editClearSelection()
	}
	return true
}

func (m *model) handleRawScrollbarMouseRelease(msg tea.MouseReleaseMsg) bool {
	if m.scrollbarDrag != scrollbarDragRaw {
		return false
	}
	m.scrollbarDrag = scrollbarDragNone
	m.scrollbarDragGrab = 0
	return true
}

func readClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.ReadClipboard()
	}
}

func requestBackgroundColorCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.RequestBackgroundColor()
	}
}

func requestForegroundColorCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.RequestForegroundColor()
	}
}

func requestCursorColorCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.RequestCursorColor()
	}
}

func requestTermcapProbeCmd() tea.Cmd {
	return tea.Batch(
		tea.RequestCapability("RGB"),
		tea.RequestCapability("Tc"),
	)
}

func requestTerminalColorsCmd() tea.Cmd {
	return tea.Batch(
		requestBackgroundColorCmd(),
		requestForegroundColorCmd(),
		requestCursorColorCmd(),
	)
}

func (m model) supportsAdvancedRendering() bool {
	if !m.colorProfileKnown {
		return true
	}
	return m.colorProfile >= colorprofile.ANSI256
}

func (m model) effectiveRichPreview() bool {
	return m.richPreview && m.supportsAdvancedRendering()
}

func (m *model) activeClipboardTarget() clipboardTarget {
	switch {
	case m.opMode == opCreate || m.opMode == opRename || m.opMode == opGoToLine || m.opMode == opGoToHeading:
		return clipboardTargetPrompt
	case m.showSearch && m.showReplace && m.replaceInputFocus:
		return clipboardTargetReplace
	case m.showSearch:
		return clipboardTargetSearch
	case m.showCmdPalette:
		return clipboardTargetCmdPalette
	case (m.mode == modeRaw || m.mode == modeSplit) && m.focusRight:
		return clipboardTargetEditor
	default:
		return clipboardTargetNone
	}
}

func (m model) currentPathForClipboard() string {
	if m.currentPath == "" {
		return ""
	}
	if rel, err := filepath.Rel(m.dir, m.currentPath); err == nil {
		return rel
	}
	return m.currentPath
}

func slugifyHeading(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

func (m model) currentHeadingLinkForClipboard() string {
	path := m.currentPathForClipboard()
	if path == "" {
		return ""
	}
	if m.currentHeading < 0 || m.currentHeading >= len(m.headings) {
		return path
	}
	anchor := slugifyHeading(m.headings[m.currentHeading].title)
	if anchor == "" {
		return path
	}
	return path + "#" + anchor
}

func (m model) currentEditorLineForClipboard() string {
	if m.mode != modeRaw && m.mode != modeSplit {
		return ""
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	idx := m.textarea.Line()
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}
