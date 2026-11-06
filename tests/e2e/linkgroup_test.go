//go:build e2e

package e2e

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// The link section's group checks (tests/e2e.sh's gp_*), on links: the
// client publishes a port of its own first, so each of the three has one
// the others reach. The client's own link to the first target, db, names
// its group links too.

// gpJoin: a server that doesn't start fails this check alone: the three
// still join, for the checks after.
func gpJoin(l *link) error {
	s := l.s
	na := l.x + "app"
	if _, err := s.DevvmNames(l.pc, "link", "publish", l.p(6)); err != nil {
		return errors.New("devvm link publish " + l.p(6))
	}
	started := lkStart(l, l.vc, "serve", "127.0.0.1", l.p(6), na+"-"+l.p(6))
	return errors.Join(gpJoined(l), started)
}

// gpDv is devvm args in dir in names mode, which must go on, saying
// lines.
func gpDv(s *S, dir string, lines []string, args ...string) error {
	r, err := s.DevvmNames(dir, args...)
	if err != nil {
		return err
	}
	return said(r, lines...)
}

// gpRefusedSaying is devvm args in dir in names mode, which must fail,
// saying line.
func gpRefusedSaying(s *S, dir, line string, args ...string) error {
	r, err := s.DevvmNames(dir, args...)
	if err == nil {
		return errors.New("devvm " + strings.Join(args, " ") + " went on")
	}
	return said(r, line)
}

func gpJoined(l *link) error {
	s := l.s
	n1, na, g := l.x+"db", l.x+"app", l.g
	trust := "group_trust group=" + g
	if err := gpDv(s, l.pc, []string{"==> group_made group=" + g + " vm=" + l.vc, trust}, "group", "join", g); err != nil {
		return err
	}
	if err := gpDv(s, l.p1, []string{
		"==> group_joined group=" + g + " vm=" + l.v1, trust,
		"==> group_links_added group=" + g + " links=db:80," + l.p(1) + "-" + l.p(2) + "," + l.p(4) + " vm=" + l.vc,
		"==> group_links_added group=" + g + " links=" + na + ":" + l.p(6) + " vm=" + l.v1,
	}, "group", "join", g); err != nil {
		return err
	}
	if err := gpDv(s, l.p2, []string{
		"==> group_joined group=" + g + " vm=" + l.v2,
		"==> group_links_added group=" + g + " links=" + l.n2 + ":" + l.p(0) + " vm=" + l.v1,
		"==> group_links_added group=" + g + " links=" + l.n2 + ":" + l.p(0) + " vm=" + l.vc,
		"==> group_links_added group=" + g + " links=" + na + ":" + l.p(6) + ", " + n1 + ":80," + l.p(0) + "-" + l.p(2) + "," + l.p(4) + " vm=" + l.v2,
	},
		"group", "join", g); err != nil {
		return err
	}
	for _, w := range []struct{ vm, host, port, word string }{
		{l.v2, n1, l.p(0), l.w1 + "-" + l.p(0)},
		{l.v2, na, l.p(6), na + "-" + l.p(6)},
		{l.v1, l.n2, l.p(0), l.w2 + "-" + l.p(0)},
		{l.v1, na, l.p(6), na + "-" + l.p(6)},
		{l.vc, l.n2, l.p(0), l.w2 + "-" + l.p(0)},
		{l.vc, "db", l.p(4), l.w1 + "-" + l.p(4)},
	} {
		if err := s.Until(15*time.Second, func() error { return lkGets(s, w.vm, w.host, w.port, w.word) }); err != nil {
			return err
		}
	}
	if err := lkGets(s, l.vc, "db", l.p(0), l.w1+"-"+l.p(0)); err != nil {
		return err
	}
	q := regexp.QuoteMeta
	return not(!lkShows(s, l.pc, `^`+q(l.n2)+` +`+l.p(0)+` +`+q(l.v2)+` +`+q(l.p2)+` +Running +yes$`, "link"), "devvm link doesn't list "+l.n2)
}

func gpShow(l *link) error {
	s := l.s
	r, _ := s.Devvm(l.pc, "group")
	Note(s, "%s", r.Stdout)
	q := regexp.QuoteMeta
	switch {
	case !Matches(r.Stdout, `^`+q(l.x)+`db +`+q(l.v1)+` +`+q(l.p1)+` +80,`+l.p(0)+`-`+l.p(2)+`,`+l.p(4)+`$`),
		!Matches(r.Stdout, `^`+q(l.n2)+` +`+q(l.v2)+` +`+q(l.p2)+` +`+l.p(0)+`$`),
		!Matches(r.Stdout, `^`+q(l.x)+`app +`+q(l.vc)+` +`+q(l.pc)+` +`+l.p(6)+`$`):
		return errors.New("devvm group doesn't list the three")
	case !lkShows(s, l.pc, `^  "version": 1,$`, "group", "--json"):
		return errors.New(`devvm group --json has no "version": 1`)
	case !lkShows(s, l.p2, `^  "group": "`+q(l.g)+`",$`, "group", "--json"):
		return errors.New("devvm group --json doesn't name the group")
	}
	return nil
}

func gpPublish(l *link) error {
	s := l.s
	w := l.w2 + "-" + l.p(5)
	if err := lkStart(l, l.v2, "serve", "127.0.0.1", l.p(5), w); err != nil {
		return err
	}
	added := "==> group_links_added group=" + l.g + " links=" + l.n2 + ":" + l.p(5)
	if err := gpDv(s, l.p2, []string{added + " vm=" + l.v1, added + " vm=" + l.vc}, "link", "publish", l.p(5)); err != nil {
		return err
	}
	if err := s.Until(15*time.Second, func() error { return lkGets(s, l.v1, l.n2, l.p(5), w) }); err != nil {
		return err
	}
	return s.Until(15*time.Second, func() error { return lkGets(s, l.vc, l.n2, l.p(5), w) })
}

// gpRefused: only a member is refused: outside the group, network
// allowlist would turn the second target's policy on, failing the checks
// after this one that need it off.
func gpRefused(l *link) error {
	s := l.s
	g := l.g
	if s.Paths.State(l.v2)["links.group"] != g {
		return errors.New(l.v2 + " isn't in " + g + ": network allowlist not tried")
	}
	if err := gpRefusedSaying(s, l.p2, "devvm: group_tier_fixed group="+g+" vm="+l.v2, "network", "allowlist"); err != nil {
		return err
	}
	if s.Paths.State(l.v2)["net.tier"] != "open" {
		return errors.New(l.v2 + " isn't on open")
	}
	if err := gpRefusedSaying(s, l.p2, "devvm: group_other group="+g+" vm="+l.v2, "group", "join", l.x+"other"); err != nil {
		return err
	}
	return gpRefusedSaying(s, l.pc, "devvm: link_from_group group="+g+" name="+l.n2+" port="+l.p(0)+" vm="+l.vc, "link", "rm", l.n2, l.p(0))
}

// gpLeave: each leaves, whatever comes of the checks, so the sections
// after run without a group.
func gpLeave(l *link) error {
	s := l.s
	g := l.g
	removed := "==> group_links_removed group=" + g + " links="
	err := gpDv(s, l.p2, []string{
		"==> group_left group=" + g + " vm=" + l.v2,
		removed + l.n2 + ":" + l.p(0) + "," + l.p(5) + " vm=" + l.v1,
		removed + l.n2 + ":" + l.p(0) + "," + l.p(5) + " vm=" + l.vc,
		removed + l.x + "app:" + l.p(6) + ", " + l.x + "db:80," + l.p(0) + "-" + l.p(2) + "," + l.p(4) + " vm=" + l.v2,
	},
		"group", "leave")
	if err == nil {
		for _, w := range []struct{ vm, name string }{{l.v1, l.n2}, {l.vc, l.n2}, {l.v2, l.x + "db"}} {
			if err = s.Until(15*time.Second, func() error { return not(lkResolves(s, w.vm, w.name), w.name+" resolves in "+w.vm) }); err != nil {
				break
			}
		}
	}
	_, e1 := s.DevvmNames(l.p1, "group", "leave")
	e2 := gpDv(s, l.pc, []string{"==> group_gone group=" + g}, "group", "leave")
	err = errors.Join(err, e1, e2)
	// The client's own link to db stays; the group's to db's other ports go.
	if err == nil {
		err = lkGets(s, l.vc, "db", l.p(0), l.w1+"-"+l.p(0))
	}
	if err == nil {
		addr := "127.77.0.1:" + l.p(4)
		err = s.Until(15*time.Second, func() error { return not(lkListening(s, l.vc, addr), addr+" still listens") })
	}
	if err == nil && (!lkShows(s, l.pc, `^db +`+l.p(0)+` +`+regexp.QuoteMeta(l.v1)+` `, "link") || lkShows(s, l.pc, `^db +`+l.p(4)+` `, "link")) {
		err = errors.New("devvm link doesn't show the client's own link alone")
	}
	_, _ = s.DevvmNames(l.p2, "link", "unpublish", l.p(5))
	_ = lkEnd(s, l.v2, "127.0.0.1", l.p(5))
	_, _ = s.DevvmNames(l.pc, "link", "unpublish", l.p(6))
	_ = lkEnd(s, l.vc, "127.0.0.1", l.p(6))
	return err
}

// gpHostname: the client alone in a group as db, the first target's
// folder part: the target is refused, as itself or another name, and
// stays out. The client leaves whatever comes of the checks.
func gpHostname(l *link) error {
	s := l.s
	g, n1 := l.x+"h", l.x+"db"
	err := gpDv(s, l.pc, []string{"==> group_made group=" + g + " vm=" + l.vc}, "group", "join", g, "--as", n1)
	refused := "devvm: group_name_hostname group=" + g + " member=" + l.vc + " name=" + n1 + " vm=" + l.v1
	if err == nil {
		err = gpRefusedSaying(s, l.p1, refused, "group", "join", g)
	}
	if err == nil {
		err = gpRefusedSaying(s, l.p1, refused, "group", "join", g, "--as", n1+"2")
	}
	if err == nil && s.Paths.State(l.v1)["links.group"] == g {
		err = errors.New(l.v1 + " is in " + g)
	}
	return errors.Join(err, gpDv(s, l.pc, []string{"==> group_gone group=" + g}, "group", "leave"))
}

// gpPolicy: all or none, with the client and the first target with
// network policy and the second without.
func gpPolicy(l *link) error {
	s := l.s
	on, off := l.x+"on", l.x+"off"
	err := gpDv(s, l.pc, []string{"==> group_made group=" + on + " vm=" + l.vc}, "group", "join", on)
	if err == nil {
		err = gpRefusedSaying(s, l.p2, "devvm: group_tier_mismatch group="+on+" tier=allowlist vm="+l.v2+" vm_tier=open", "group", "join", on)
	}
	if err == nil {
		_, err = s.DevvmNames(l.p2, "group", "join", off)
	}
	if err == nil {
		err = gpRefusedSaying(s, l.p1, "devvm: group_tier_mismatch group="+off+" tier=open vm="+l.v1+" vm_tier=allowlist", "group", "join", off)
	}
	if err == nil && s.Paths.State(l.v1)["links.group"] == off {
		err = errors.New(l.v1 + " is in " + off)
	}
	// The client's own links to db are $P47240 and $P47242 (lkProxyAdded).
	if err == nil {
		err = gpDv(s, l.p1, []string{"==> group_links_added group=" + on + " links=db:80," + l.p(1) + "," + l.p(4) + " vm=" + l.vc}, "group", "join", on)
	}
	if err == nil {
		err = s.Until(15*time.Second, func() error { return lkGets(s, l.vc, "db", "80", l.w1+"-80") })
	}
	if err == nil {
		err = lkCurls(s, l.vc, "http://db:80/", l.w1+"-80")
	}
	for _, d := range []string{l.p1, l.pc, l.p2} {
		_, _ = s.DevvmNames(d, "group", "leave")
	}
	if err != nil {
		return err
	}
	for _, v := range []string{l.v1, l.vc, l.v2} {
		if s.Paths.State(v)["links.group"] != "" {
			return errors.New(v + " is still in a group")
		}
	}
	return nil
}
