package tabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/eventstore/slicestore"
	"fiatjaf.com/nostr/khatru"
	"fiatjaf.com/nostr/khatru/blossom"
	"fiatjaf.com/nostr/khatru/grasp"
	"github.com/guigui-gui/guigui"
	"github.com/guigui-gui/guigui/basicwidget"

	"github.com/1l0/gnak/internal/state"
)

const (
	serveHost    = "127.0.0.1"
	servePort    = 10547
	maxServeLogs = 400
)

type Serve struct {
	guigui.DefaultWidget

	state *state.AppState

	negentropyLabel    basicwidget.Text
	negentropyToggle   basicwidget.Toggle
	blossomLabel       basicwidget.Text
	blossomToggle      basicwidget.Toggle
	graspLabel         basicwidget.Text
	graspToggle        basicwidget.Toggle
	serverLabel        basicwidget.Text
	serverAddressInput basicwidget.TextInput

	startButton basicwidget.Button
	stopButton  basicwidget.Button

	logsLabel basicwidget.Text
	logsList  basicwidget.List[string]

	eventsLabel      basicwidget.Text
	eventsList       basicwidget.List[string]
	eventDetailLabel basicwidget.Text
	eventDetail      basicwidget.Text

	blossomListLabel basicwidget.Text
	blossomList      basicwidget.List[string]

	graspListLabel basicwidget.Text
	graspList      basicwidget.List[string]

	relayMu     sync.Mutex
	relay       *khatru.Relay
	relayCancel context.CancelFunc

	db *slicestore.SliceStore

	blobStoreMu sync.RWMutex
	blobStore   map[string][]byte
	repoDir     string

	logsMu             sync.Mutex
	logs               []string
	logsVersion        atomic.Int64
	appliedLogsVersion int64

	eventsMu             sync.Mutex
	events               []nostr.Event
	eventsVersion        atomic.Int64
	appliedEventsVersion int64
	selectedEventID      string

	blossomMu             sync.Mutex
	blossomItems          []blobInfo
	blossomVersion        atomic.Int64
	appliedBlossomVersion int64

	graspMu             sync.Mutex
	graspItems          []repoInfo
	graspVersion        atomic.Int64
	appliedGraspVersion int64

	serverAddress atomic.Value

	relayRunning  atomic.Bool
	starting      atomic.Bool
	blossomActive atomic.Bool
	graspActive   atomic.Bool
}

type serveConfig struct {
	Negentropy bool
	Blossom    bool
	Grasp      bool
}

type blobInfo struct {
	Key  string
	Size int
}

type repoInfo struct {
	Name string
	Path string
	Size int64
	Head string
}

func NewServe(st *state.AppState) *Serve {
	return &Serve{state: st}
}

func (s *Serve) Build(_ *guigui.Context, adder *guigui.ChildAdder) error {
	for _, widget := range []guigui.Widget{
		&s.negentropyLabel,
		&s.negentropyToggle,
		&s.blossomLabel,
		&s.blossomToggle,
		&s.graspLabel,
		&s.graspToggle,
		&s.serverLabel,
		&s.serverAddressInput,
		&s.startButton,
		&s.stopButton,
		&s.logsLabel,
		&s.logsList,
		&s.eventsLabel,
		&s.eventsList,
		&s.eventDetailLabel,
		&s.eventDetail,
		&s.blossomListLabel,
		&s.blossomList,
		&s.graspListLabel,
		&s.graspList,
	} {
		adder.AddChild(widget)
	}

	s.negentropyLabel.SetValue("Negentropy")
	s.blossomLabel.SetValue("Blossom")
	s.graspLabel.SetValue("Grasp")
	s.serverLabel.SetValue("Server address")
	s.serverLabel.SetBold(true)

	s.serverAddressInput.SetEditable(false)

	s.startButton.SetText("Start")
	s.startButton.SetOnUp(s.handleStart)

	s.stopButton.SetText("Stop")
	s.stopButton.SetOnUp(s.handleStop)

	s.logsLabel.SetValue("Logs")
	s.logsLabel.SetBold(true)
	s.logsList.SetStripeVisible(true)

	s.eventsLabel.SetValue("Events")
	s.eventsLabel.SetBold(true)
	s.eventsList.SetStripeVisible(true)
	s.eventsList.SetOnItemSelected(func(index int) {
		item, ok := s.eventsList.ItemByIndex(index)
		if !ok {
			return
		}
		s.selectedEventID = item.Value
		s.showEventByID(item.Value)
	})

	s.eventDetailLabel.SetValue("Selected event JSON")
	s.eventDetailLabel.SetBold(true)
	s.eventDetail.SetAutoWrap(true)
	s.eventDetail.SetSelectable(true)
	s.eventDetail.SetTabular(true)
	s.eventDetail.SetValue("events will appear here once the relay stores them")

	s.blossomListLabel.SetValue("Blossom blobs")
	s.blossomListLabel.SetBold(true)
	s.blossomList.SetStripeVisible(true)

	s.graspListLabel.SetValue("Grasp repos")
	s.graspListLabel.SetBold(true)
	s.graspList.SetStripeVisible(true)

	s.logs = make([]string, 0, maxServeLogs)

	s.serverAddress.Store("")

	return nil
}

func (s *Serve) Layout(context *guigui.Context, widgetBounds *guigui.WidgetBounds, layouter *guigui.ChildLayouter) {
	layout := s.layoutSpec(context)
	layout.LayoutWidgets(context, widgetBounds.Bounds(), layouter)
}

func (s *Serve) Measure(context *guigui.Context, constraints guigui.Constraints) image.Point {
	layout := s.layoutSpec(context)
	return layout.Measure(context, constraints)
}

func (s *Serve) layoutSpec(context *guigui.Context) guigui.LinearLayout {
	u := basicwidget.UnitSize(context)

	optionLayout := func(label *basicwidget.Text, toggle *basicwidget.Toggle) guigui.LinearLayoutItem {
		return guigui.LinearLayoutItem{
			Layout: guigui.LinearLayout{
				Direction: guigui.LayoutDirectionHorizontal,
				Gap:       u / 4,
				Items: []guigui.LinearLayoutItem{
					{Widget: label},
					{Widget: toggle, Size: guigui.FixedSize(3 * u)},
				},
			},
			Size: guigui.FixedSize(8 * u),
		}
	}

	optionsRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			optionLayout(&s.negentropyLabel, &s.negentropyToggle),
			optionLayout(&s.blossomLabel, &s.blossomToggle),
			optionLayout(&s.graspLabel, &s.graspToggle),
			{
				Layout: guigui.LinearLayout{
					Direction: guigui.LayoutDirectionVertical,
					Items: []guigui.LinearLayoutItem{
						{Widget: &s.serverLabel},
						{Widget: &s.serverAddressInput},
					},
				},
				Size: guigui.FlexibleSize(1),
			},
		},
	}

	buttonsRow := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionHorizontal,
		Gap:       u / 2,
		Items: []guigui.LinearLayoutItem{
			{Widget: &s.startButton, Size: guigui.FixedSize(8 * u)},
			{Widget: &s.stopButton, Size: guigui.FixedSize(6 * u)},
		},
	}

	eventsColumn := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 4,
		Items: []guigui.LinearLayoutItem{
			{Widget: &s.eventsLabel},
			{Widget: &s.eventsList, Size: guigui.FlexibleSize(3)},
			{Widget: &s.eventDetailLabel},
			{Widget: &s.eventDetail, Size: guigui.FlexibleSize(2)},
		},
	}

	blossomColumn := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 4,
		Items: []guigui.LinearLayoutItem{
			{Widget: &s.blossomListLabel},
			{Widget: &s.blossomList, Size: guigui.FlexibleSize(1)},
		},
	}

	graspColumn := guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 4,
		Items: []guigui.LinearLayoutItem{
			{Widget: &s.graspListLabel},
			{Widget: &s.graspList, Size: guigui.FlexibleSize(1)},
		},
	}

	bottomItems := []guigui.LinearLayoutItem{{Layout: eventsColumn, Size: guigui.FlexibleSize(2)}}
	if s.shouldShowBlossomPanel() {
		bottomItems = append(bottomItems, guigui.LinearLayoutItem{Layout: blossomColumn, Size: guigui.FlexibleSize(1)})
	}
	if s.shouldShowGraspPanel() {
		bottomItems = append(bottomItems, guigui.LinearLayoutItem{Layout: graspColumn, Size: guigui.FlexibleSize(1)})
	}

	return guigui.LinearLayout{
		Direction: guigui.LayoutDirectionVertical,
		Gap:       u / 2,
		Padding:   guigui.Padding{Start: u, End: u, Top: u, Bottom: u},
		Items: []guigui.LinearLayoutItem{
			{Layout: optionsRow},
			{Layout: buttonsRow},
			{Widget: &s.logsLabel},
			{Widget: &s.logsList, Size: guigui.FixedSize(5 * u)},
			{
				Layout: guigui.LinearLayout{
					Direction: guigui.LayoutDirectionHorizontal,
					Gap:       u / 2,
					Items:     bottomItems,
				},
				Size: guigui.FlexibleSize(1),
			},
		},
	}
}

func (s *Serve) shouldShowBlossomPanel() bool {
	if s.relayRunning.Load() && s.blossomActive.Load() {
		return true
	}
	return s.blossomToggle.Value()
}

func (s *Serve) shouldShowGraspPanel() bool {
	if s.relayRunning.Load() && s.graspActive.Load() {
		return true
	}
	return s.graspToggle.Value()
}

func (s *Serve) Tick(context *guigui.Context, _ *guigui.WidgetBounds) error {
	running := s.relayRunning.Load()
	busy := running || s.starting.Load()
	context.SetEnabled(&s.startButton, !busy)
	context.SetEnabled(&s.stopButton, running)
	context.SetEnabled(&s.negentropyToggle, !busy)
	context.SetEnabled(&s.blossomToggle, !busy)
	context.SetEnabled(&s.graspToggle, !busy)

	if version := s.logsVersion.Load(); version != s.appliedLogsVersion {
		s.refreshLogs()
		s.appliedLogsVersion = version
	}
	if version := s.eventsVersion.Load(); version != s.appliedEventsVersion {
		s.refreshEvents()
		s.appliedEventsVersion = version
	}
	if version := s.blossomVersion.Load(); version != s.appliedBlossomVersion {
		s.refreshBlossom()
		s.appliedBlossomVersion = version
	}
	if version := s.graspVersion.Load(); version != s.appliedGraspVersion {
		s.refreshGrasp()
		s.appliedGraspVersion = version
	}

	if addr, ok := s.serverAddress.Load().(string); ok {
		if s.serverAddressInput.Value() != addr {
			s.serverAddressInput.ForceSetValue(addr)
		}
	}

	return nil
}

func (s *Serve) handleStart() {
	if s.relayRunning.Load() || s.starting.Load() {
		return
	}
	cfg := serveConfig{
		Negentropy: s.negentropyToggle.Value(),
		Blossom:    s.blossomToggle.Value(),
		Grasp:      s.graspToggle.Value(),
	}
	s.starting.Store(true)
	s.state.SetStatus("starting local relay...")
	guigui.RequestRedraw(s)
	go s.runRelay(cfg)
}

func (s *Serve) handleStop() {
	s.state.SetStatus("stopping local relay")
	s.stopRelay()
}

func (s *Serve) runRelay(cfg serveConfig) {
	defer func() {
		s.starting.Store(false)
		guigui.RequestRedraw(s)
	}()

	s.ensureDB()
	relay := khatru.NewRelay()
	relay.Info.Name = "gnak serve"
	relay.Info.Description = "a local relay for testing, debugging and development."
	relay.Info.Software = "https://github.com/1l0/gnak"
	relay.Info.Version = "dev"
	relay.UseEventstore(s.db, 500)

	if cfg.Negentropy {
		relay.Negentropy = true
	}

	_, cancel := context.WithCancel(context.Background())
	s.relayMu.Lock()
	s.relay = relay
	s.relayCancel = cancel
	s.relayMu.Unlock()

	if cfg.Blossom {
		s.setupBlossom(relay)
	} else {
		s.blossomActive.Store(false)
		s.clearBlossomItems()
	}
	if cfg.Grasp {
		if err := s.setupGrasp(relay); err != nil {
			s.appendLog("failed to initialize grasp: %v", err)
			s.graspActive.Store(false)
		}
	} else {
		s.graspActive.Store(false)
		s.clearGraspItems()
	}

	started := make(chan bool, 1)
	exited := make(chan error, 1)

	relay.OnRequest = func(ctx context.Context, filter nostr.Filter) (bool, string) {
		negentropy := ""
		if khatru.IsNegentropySession(ctx) {
			negentropy = "negentropy "
		}
		s.appendLog("%srequest: %s", negentropy, filter)
		return false, ""
	}
	relay.OnCount = func(ctx context.Context, filter nostr.Filter) (bool, string) {
		s.appendLog("count request: %s", filter)
		return false, ""
	}
	relay.OnEvent = func(ctx context.Context, event nostr.Event) (bool, string) {
		s.appendLog("event: %s", event.ID.Hex())
		return false, ""
	}
	relay.OnEventSaved = func(ctx context.Context, event nostr.Event) {
		s.appendLog("event saved: %s", event.ID.Hex())
		s.refreshEventsFromStore()
	}

	totalConnections := atomic.Int32{}
	relay.OnConnect = func(ctx context.Context) {
		totalConnections.Add(1)
		s.appendLog("client connected (%d active)", totalConnections.Load())
		go func() {
			<-ctx.Done()
			remaining := totalConnections.Add(-1)
			s.appendLog("client disconnected (%d active)", remaining)
		}()
	}

	go func() {
		exited <- relay.Start(serveHost, servePort, started)
	}()

	select {
	case <-started:
		addr := fmt.Sprintf("ws://%s:%d", serveHost, servePort)
		s.serverAddress.Store(addr)
		s.relayRunning.Store(true)
		s.state.SetStatus("serve relay running")
		s.appendLog("relay running at %s", addr)
		if cfg.Grasp && s.repoDir != "" {
			s.appendLog("grasp repos at %s", s.repoDir)
		}
		guigui.RequestRedraw(s)
	case err := <-exited:
		if err != nil {
			s.appendLog("relay failed to start: %v", err)
		} else {
			s.appendLog("relay exited")
		}
		s.cleanupAfterStop()
		s.state.SetStatus("serve relay stopped")
		return
	}

	if err := <-exited; err != nil {
		s.appendLog("relay exited with error: %v", err)
	} else {
		s.appendLog("relay stopped")
	}
	s.cleanupAfterStop()
	s.state.SetStatus("serve relay stopped")
}

func (s *Serve) stopRelay() {
	s.relayMu.Lock()
	relay := s.relay
	cancel := s.relayCancel
	s.relay = nil
	s.relayCancel = nil
	s.relayMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if relay != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		relay.Shutdown(ctx)
	}
}

func (s *Serve) cleanupAfterStop() {
	s.relayRunning.Store(false)
	s.serverAddress.Store("")
	s.blossomActive.Store(false)
	s.graspActive.Store(false)
	s.clearBlossomItems()
	s.clearGraspItems()
	s.guaranteeEventDetailMessage()
	guigui.RequestRedraw(s)
}

func (s *Serve) guaranteeEventDetailMessage() {
	if s.selectedEventID == "" {
		s.eventDetail.SetValue("events will appear here once the relay stores them")
	}
}

func (s *Serve) ensureDB() {
	if s.db == nil {
		s.db = &slicestore.SliceStore{}
	}
}

func (s *Serve) refreshEventsFromStore() {
	if s.db == nil {
		return
	}
	events := make([]nostr.Event, 0, 512)
	for evt := range s.db.QueryEvents(nostr.Filter{}, 5000) {
		events = append(events, evt)
	}
	s.eventsMu.Lock()
	s.events = events
	s.eventsMu.Unlock()
	s.eventsVersion.Add(1)
	guigui.RequestRedraw(s)
}

func (s *Serve) setupBlossom(relay *khatru.Relay) {
	s.blobStoreMu.Lock()
	if s.blobStore == nil {
		s.blobStore = make(map[string][]byte)
	}
	s.blobStoreMu.Unlock()

	bs := blossom.New(relay, fmt.Sprintf("http://%s:%d", serveHost, servePort))
	bs.Store = blossom.NewMemoryBlobIndex()

	bs.StoreBlob = func(ctx context.Context, sha256 string, ext string, body []byte) error {
		key := sha256 + ext
		data := append([]byte(nil), body...)
		s.blobStoreMu.Lock()
		s.blobStore[key] = data
		s.blobStoreMu.Unlock()
		s.appendLog("blob stored: %s", key)
		s.captureBlossomBlobs()
		return nil
	}
	bs.LoadBlob = func(ctx context.Context, sha256 string, ext string) (io.ReadSeeker, *url.URL, error) {
		key := sha256 + ext
		s.blobStoreMu.RLock()
		data, ok := s.blobStore[key]
		s.blobStoreMu.RUnlock()
		if !ok {
			s.appendLog("blob miss: %s", key)
			return nil, nil, os.ErrNotExist
		}
		s.appendLog("blob download: %s", key)
		return bytes.NewReader(data), nil, nil
	}
	bs.DeleteBlob = func(ctx context.Context, sha256 string, ext string) error {
		key := sha256 + ext
		s.blobStoreMu.Lock()
		delete(s.blobStore, key)
		s.blobStoreMu.Unlock()
		s.appendLog("blob delete: %s", key)
		s.captureBlossomBlobs()
		return nil
	}

	s.blossomActive.Store(true)
	s.captureBlossomBlobs()
}

func (s *Serve) setupGrasp(relay *khatru.Relay) error {
	if s.repoDir == "" {
		dir, err := os.MkdirTemp("", "gnak-serve-grasp-")
		if err != nil {
			return err
		}
		s.repoDir = dir
	}
	g := grasp.New(relay, s.repoDir)
	g.OnRead = func(ctx context.Context, pubkey nostr.PubKey, repo string) (bool, string) {
		s.appendLog("git read by '%s' at '%s'", pubkey.Hex(), repo)
		s.captureGraspRepos()
		return false, ""
	}
	g.OnWrite = func(ctx context.Context, pubkey nostr.PubKey, repo string) (bool, string) {
		s.appendLog("git write by '%s' at '%s'", pubkey.Hex(), repo)
		s.captureGraspRepos()
		return false, ""
	}
	s.graspActive.Store(true)
	s.captureGraspRepos()
	return nil
}

func (s *Serve) captureBlossomBlobs() {
	s.blobStoreMu.RLock()
	blobs := make([]blobInfo, 0, len(s.blobStore))
	for key, value := range s.blobStore {
		blobs = append(blobs, blobInfo{Key: key, Size: len(value)})
	}
	s.blobStoreMu.RUnlock()
	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Key < blobs[j].Key
	})
	s.blossomMu.Lock()
	s.blossomItems = blobs
	s.blossomMu.Unlock()
	s.blossomVersion.Add(1)
	guigui.RequestRedraw(s)
}

func (s *Serve) captureGraspRepos() {
	repos := make([]repoInfo, 0)
	if s.repoDir != "" {
		entries, err := os.ReadDir(s.repoDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				path := filepath.Join(s.repoDir, name)
				size := calculateDirSize(path)
				head := getHeadCommit(path)
				repos = append(repos, repoInfo{Name: name, Path: path, Size: size, Head: head})
			}
		}
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})
	s.graspMu.Lock()
	s.graspItems = repos
	s.graspMu.Unlock()
	s.graspVersion.Add(1)
	guigui.RequestRedraw(s)
}

func (s *Serve) refreshLogs() {
	s.logsMu.Lock()
	items := make([]basicwidget.ListItem[string], len(s.logs))
	for i, line := range s.logs {
		items[i] = basicwidget.ListItem[string]{
			Text:  line,
			Value: fmt.Sprintf("%d", len(s.logs)-i),
		}
	}
	s.logsMu.Unlock()
	s.logsList.SetItems(items)
}

func (s *Serve) refreshEvents() {
	s.eventsMu.Lock()
	items := make([]basicwidget.ListItem[string], len(s.events))
	selectedID := s.selectedEventID
	for i, evt := range s.events {
		timestamp := time.Unix(int64(evt.CreatedAt), 0).UTC().Format(time.DateTime)
		summary := fmt.Sprintf("%s | kind %d | %s", timestamp, evt.Kind, evt.PubKey.Hex())
		items[i] = basicwidget.ListItem[string]{
			Text:  summary,
			Value: evt.ID.Hex(),
		}
	}
	s.eventsMu.Unlock()
	s.eventsList.SetItems(items)
	if len(items) == 0 {
		s.selectedEventID = ""
		s.eventDetail.SetValue("events will appear here once the relay stores them")
		return
	}
	if selectedID == "" {
		selectedID = items[0].Value
		s.selectedEventID = selectedID
		s.eventsList.SelectItemByIndex(0)
	} else {
		s.eventsList.SelectItemByValue(selectedID)
	}
	s.showEventByID(selectedID)
}

func (s *Serve) refreshBlossom() {
	s.blossomMu.Lock()
	items := make([]basicwidget.ListItem[string], len(s.blossomItems))
	for i, blob := range s.blossomItems {
		items[i] = basicwidget.ListItem[string]{
			Text:  fmt.Sprintf("%s (%d bytes)", blob.Key, blob.Size),
			Value: blob.Key,
		}
	}
	s.blossomMu.Unlock()
	s.blossomList.SetItems(items)
}

func (s *Serve) refreshGrasp() {
	s.graspMu.Lock()
	items := make([]basicwidget.ListItem[string], len(s.graspItems))
	for i, repo := range s.graspItems {
		text := fmt.Sprintf("repo: %s\npath: %s\nsize: %d bytes\nhead: %s", repo.Name, repo.Path, repo.Size, repo.Head)
		items[i] = basicwidget.ListItem[string]{
			Text:  text,
			Value: repo.Path,
		}
	}
	s.graspMu.Unlock()
	s.graspList.SetItems(items)
}

func (s *Serve) showEventByID(id string) {
	if id == "" {
		s.eventDetail.SetValue("events will appear here once the relay stores them")
		return
	}
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	for _, evt := range s.events {
		if evt.ID.Hex() != id {
			continue
		}
		pretty, err := json.MarshalIndent(evt, "", "  ")
		if err != nil {
			s.eventDetail.SetValue("failed to encode event: " + err.Error())
			return
		}
		s.eventDetail.SetValue(string(pretty))
		return
	}
	s.eventDetail.SetValue("event not found (it may have been pruned)")
}

func (s *Serve) appendLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	s.logsMu.Lock()
	s.logs = append([]string{msg}, s.logs...)
	if len(s.logs) > maxServeLogs {
		s.logs = s.logs[:maxServeLogs]
	}
	s.logsMu.Unlock()
	s.logsVersion.Add(1)
	guigui.RequestRedraw(s)
}

func (s *Serve) clearBlossomItems() {
	s.blossomMu.Lock()
	s.blossomItems = s.blossomItems[:0]
	s.blossomMu.Unlock()
	s.blossomVersion.Add(1)
	guigui.RequestRedraw(s)
}

func (s *Serve) clearGraspItems() {
	s.graspMu.Lock()
	s.graspItems = s.graspItems[:0]
	s.graspMu.Unlock()
	s.graspVersion.Add(1)
	guigui.RequestRedraw(s)
}

func calculateDirSize(path string) int64 {
	var size int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

func getHeadCommit(repoPath string) string {
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "no commits"
	}
	return strings.TrimSpace(string(out))
}
