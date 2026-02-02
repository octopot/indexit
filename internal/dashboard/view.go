package dashboard

import (
	"cmp"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/stats"
)

// view is what the page shows. Numbers come formatted; the template only
// places them.
type view struct {
	Assets map[string]string
	// Here names this machine in the copy.
	Here string
	// Restart is the command that starts the service again.
	Restart string
	// ServiceLog says where the service writes its log.
	ServiceLog string
	Ready      bool
	Version    string
	Updated    time.Time
	Origin     string
	DataDir    string
	ImageDir   string
	Config     string
	ScanErr    string
	LogErr     string

	Saved     int
	SavedText string
	Invalid   int
	Size      amount
	Free      amount
	HasFree   bool

	Served    amount
	Sent      amount
	Downloads int
	Reuse     string
	Since     time.Time
	Gone      int
	Failed    int
	Dropped   int64

	Dialogs  string
	Activity activity
	Top      []channel
	Recent   []recent
}

// amount is a formatted number with its unit apart, so the digits lead.
type amount struct {
	Value, Unit string
}

type channel struct {
	Title, Link string
	Photos      string
	Share       string
	Bar         float64
}

type recent struct {
	URL, Thumbnail, Telegram string
	Title, Message, Size     string
	Saved                    time.Time
	Alt                      string
	Eager                    bool
}

type activity struct {
	// Calendar is set once the oldest file is two weeks old.
	Calendar *calendar
	First    time.Time
	Days     int
	Busiest  time.Time
	Peak     string
	Active   string
}

type calendar struct {
	Weeks  []week
	Months []month
	Label  string
	// Table holds the same days as text, a week a row.
	Table []tableWeek
}

type tableWeek struct {
	Week   string
	Counts []string
}

type week struct {
	X    int
	Days []day
}

type day struct {
	Y     int
	Level int
	Title string
}

type month struct {
	X    int
	Name string
}

const (
	calendarWeeks = 26
	cell          = 12
	gap           = 3
)

// build turns an inventory and the log totals into the page.
func (d *Dashboard) build(inv *Inventory, totals stats.Totals, now time.Time) view {
	v := view{
		Assets:     d.assets.paths,
		Here:       here(),
		Restart:    restart(),
		ServiceLog: serviceLog(),
		Version:    d.Version,
		Origin:     d.Store.Origin,
		DataDir:    home(d.Store.Root),
		ImageDir:   home(d.Store.ImageDir()),
		Config:     home(d.ConfigPath),
		Dropped:    d.dropped(),
	}
	if v.Origin == "" {
		v.Origin = preview.DefaultOrigin
	}
	if inv == nil {
		return v
	}
	v.Ready = true
	v.Updated = inv.At
	if inv.Err != nil {
		v.ScanErr = inv.Err.Error()
	}
	v.Saved = len(inv.Photos)
	v.SavedText = count(int64(v.Saved))
	v.Invalid = inv.Invalid
	var size int64
	for _, photo := range inv.Photos {
		size += photo.Size
	}
	v.Size = bytesAmount(size)
	// the disk of the photographs, which may be another than of the rest
	if free, ok := diskFree(d.Store.ImageDir()); ok {
		v.Free, v.HasFree = bytesAmount(int64(min(free, math.MaxInt64))), true
	}

	v.Served = countAmount(totals.Served)
	v.Sent = bytesAmount(totals.Sent)
	v.Downloads = int(totals.Downloads)
	if totals.Downloads > 0 && totals.Served > totals.Downloads {
		v.Reuse = strconv.FormatFloat(float64(totals.Served)/float64(totals.Downloads), 'f', 1, 64)
		v.Reuse = strings.TrimSuffix(v.Reuse, ".0")
	}
	v.Since = totals.Since
	for status, n := range totals.Failures {
		if status != http.StatusGone {
			v.Failed += int(n)
		}
	}
	saved := make(map[string]bool, len(inv.Photos))
	for _, photo := range inv.Photos {
		saved[photo.Source.Path()] = true
	}
	for path := range totals.Gone {
		if !saved[path] {
			v.Gone++
		}
	}
	v.Activity = buildActivity(inv.Photos, now)
	v.Top = buildChannels(inv)
	dialogs := map[string]bool{}
	for _, photo := range inv.Photos {
		dialogs[peerKey(photo.Source)] = true
	}
	if len(dialogs) > 0 {
		v.Dialogs = plural(len(dialogs), "channel")
	}
	for i, photo := range inv.Photos[:min(10, len(inv.Photos))] {
		title := peerTitle(inv, photo.Source)
		v.Recent = append(v.Recent, recent{
			URL:       d.Store.URL(photo.Source),
			Thumbnail: "/thumbnails/" + strings.TrimPrefix(photo.Source.Path(), preview.Prefix),
			Telegram:  telegramLink(inv, photo.Source),
			Title:     title,
			Message:   "Message " + strconv.Itoa(photo.Source.Message),
			Size:      bytesText(photo.Size),
			Saved:     photo.Saved,
			Alt:       "Photograph of message " + strconv.Itoa(photo.Source.Message) + " in " + title + ", saved " + photo.Saved.Format("2 Jan 2006"),
			Eager:     i < 5,
		})
	}
	return v
}

func buildActivity(photos []Photo, now time.Time) activity {
	var a activity
	if len(photos) == 0 {
		return a
	}
	perDay := map[time.Time]int{}
	for _, photo := range photos {
		day := midnight(photo.Saved)
		perDay[day]++
		if a.First.IsZero() || day.Before(a.First) {
			a.First = day
		}
	}
	for day, n := range perDay {
		if n > perDay[a.Busiest] || n == perDay[a.Busiest] && day.After(a.Busiest) {
			a.Busiest = day
		}
	}
	a.Peak = plural(perDay[a.Busiest], "photograph")
	a.Active = plural(len(perDay), "day")
	today := midnight(now)
	a.Days = int(today.Sub(a.First).Hours()/24+0.5) + 1
	if a.Days < 14 {
		return a
	}
	// Weeks start on Monday; the last column is the current week.
	start := today.AddDate(0, 0, -((int(today.Weekday())+6)%7)-7*(calendarWeeks-1))
	levels := quartiles(perDay, start)
	c := &calendar{}
	total, days := 0, 0
	for w := range calendarWeeks {
		col := week{X: w * (cell + gap)}
		row := tableWeek{Week: start.AddDate(0, 0, w*7).Format("2 Jan"), Counts: make([]string, 7)}
		for i := range 7 {
			date := start.AddDate(0, 0, w*7+i)
			if date.After(today) {
				break
			}
			n := perDay[date]
			row.Counts[i] = strconv.Itoa(n)
			if n > 0 {
				total += n
				days++
			}
			if date.Day() == 1 || w == 0 && i == 0 {
				c.Months = append(c.Months, month{X: col.X, Name: date.Format("Jan")})
			}
			col.Days = append(col.Days, day{Y: i * (cell + gap), Level: levels(n), Title: plural(n, "photograph") + " on " + date.Format("Mon 2 Jan 2006")})
		}
		c.Weeks = append(c.Weeks, col)
		c.Table = append(c.Table, row)
	}
	// a label at the first column gives way to the first new month after it
	if len(c.Months) > 1 && c.Months[1].X-c.Months[0].X < 3*(cell+gap) {
		c.Months = c.Months[1:]
	}
	c.Label = plural(total, "photograph") + " saved on " + plural(days, "day") + " in the last 26 weeks"
	a.Calendar = c
	return a
}

// quartiles maps a day count to one of five levels: 0 for none, then by the
// quartiles of the days with photographs since start.
func quartiles(perDay map[time.Time]int, start time.Time) func(int) int {
	var counts []int
	for day, n := range perDay {
		if !day.Before(start) {
			counts = append(counts, n)
		}
	}
	slices.Sort(counts)
	return func(n int) int {
		if n == 0 || len(counts) == 0 {
			return 0
		}
		rank, _ := slices.BinarySearch(counts, n+1)
		return 1 + min(3, 4*rank/(len(counts)+1))
	}
}

func buildChannels(inv *Inventory) []channel {
	type group struct {
		src    preview.Source
		photos int
	}
	groups := map[string]*group{}
	for _, photo := range inv.Photos {
		key := peerKey(photo.Source)
		if groups[key] == nil {
			groups[key] = &group{src: photo.Source}
		}
		groups[key].photos++
	}
	sorted := make([]*group, 0, len(groups))
	for _, g := range groups {
		sorted = append(sorted, g)
	}
	slices.SortFunc(sorted, func(a, b *group) int {
		return cmp.Or(b.photos-a.photos, strings.Compare(peerTitle(inv, a.src), peerTitle(inv, b.src)))
	})
	const top = 6
	total := len(inv.Photos)
	var channels []channel
	add := func(title, link string, photos int) {
		share := float64(photos) / float64(total)
		channels = append(channels, channel{Title: title, Link: link, Photos: count(int64(photos)), Share: percent(share), Bar: math.Round(share*1000) / 10})
	}
	for i, g := range sorted {
		if i == top && len(sorted) > top+1 {
			rest := 0
			for _, g := range sorted[top:] {
				rest += g.photos
			}
			add(plural(len(sorted)-top, "other channel"), "", rest)
			break
		}
		add(peerTitle(inv, g.src), dialogLink(inv, g.src), g.photos)
	}
	return channels
}

// peerTitle names the channel of src: its title, its username or its ID.
func peerTitle(inv *Inventory, src preview.Source) string {
	peer := inv.Peers[peerKey(src)]
	switch {
	case peer.Title != "":
		return peer.Title
	case peer.Username != "":
		return "@" + peer.Username
	}
	return "Channel " + strconv.FormatInt(src.Channel, 10)
}

// dialogLink is the public address of a channel with a username.
func dialogLink(inv *Inventory, src preview.Source) string {
	if peer := inv.Peers[peerKey(src)]; peer.Username != "" {
		return "https://t.me/" + peer.Username
	}
	return ""
}

// telegramLink is the address of the post of src in Telegram:
// t.me/<username>/<id>, or t.me/c/<id>/<id> for members.
func telegramLink(inv *Inventory, src preview.Source) string {
	if peer := inv.Peers[peerKey(src)]; peer.Username != "" {
		return "https://t.me/" + peer.Username + "/" + strconv.Itoa(src.Message)
	}
	return "https://t.me/c/" + strconv.FormatInt(src.Channel, 10) + "/" + strconv.Itoa(src.Message)
}

// here names this machine: the service runs on a Mac with Homebrew, or by
// hand elsewhere.
func here() string {
	if runtime.GOOS == "darwin" {
		return "this Mac"
	}
	return "this machine"
}

func restart() string {
	if runtime.GOOS == "darwin" {
		return "brew services restart octolab/tap/indexit"
	}
	return "indexit service run"
}

func serviceLog() string {
	if runtime.GOOS == "darwin" {
		return "$(brew --prefix)/var/log/indexit.log"
	}
	return "the output of indexit service run"
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// count writes n with thousands separators.
func count(n int64) string {
	s := strconv.FormatInt(n, 10)
	start := 0
	if n < 0 {
		start = 1
	}
	for i := len(s) - 3; i > start; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// countAmount is count, in thousands or millions from 100,000 on.
func countAmount(n int64) amount {
	switch {
	case n >= 1_000_000:
		return amount{Value: short(float64(n) / 1e6), Unit: "M"}
	case n >= 100_000:
		return amount{Value: short(float64(n) / 1e3), Unit: "K"}
	}
	return amount{Value: count(n)}
}

// bytesAmount uses decimal units, as Finder does.
func bytesAmount(n int64) amount {
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	if n < 1000 {
		return amount{Value: strconv.FormatInt(n, 10), Unit: "B"}
	}
	f := float64(n)
	unit := ""
	for _, u := range units {
		f /= 1000
		unit = u
		if f < 999.95 {
			break
		}
	}
	return amount{Value: short(f), Unit: unit}
}

func bytesText(n int64) string {
	a := bytesAmount(n)
	return a.Value + " " + a.Unit
}

// short keeps one decimal below 100.
func short(f float64) string {
	if f >= 99.95 {
		return strconv.FormatFloat(math.Round(f), 'f', 0, 64)
	}
	return strings.TrimSuffix(strconv.FormatFloat(f, 'f', 1, 64), ".0")
}

func percent(f float64) string {
	if f < 0.1 {
		return strings.TrimSuffix(strconv.FormatFloat(f*100, 'f', 1, 64), ".0") + "%"
	}
	return strconv.FormatFloat(math.Round(f*100), 'f', 0, 64) + "%"
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return count(int64(n)) + " " + noun + "s"
}

// home shortens a path in the home directory to ~/….
func home(path string) string {
	if dir, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(dir, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return path
}
