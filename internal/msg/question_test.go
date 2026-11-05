package msg

import "testing"

// A question's refusal names what answers it, after the question unless
// its prompt was shown; one RefusedWith made is its own refusal alone.
func TestQuestionRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		q        Question
		shown    bool
		question string
		refusal  string
	}{
		{GCConfirm().AnsweredBy("--force"), false, "gc_confirm", "no_input answer=--force"},
		{GCConfirm().AnsweredBy("--force"), true, "", "no_input answer=--force"},
		{DeleteAnyway().RefusedWith(NoTerminalDelete("devvm rm")), false, "", "no_terminal_delete command=devvm rm"},
		{DeleteAnyway().RefusedWith(NoTerminalDelete("devvm rm")), true, "", "no_terminal_delete command=devvm rm"},
	} {
		r := tc.q.Refused(tc.shown)
		if r.Question.Names() != tc.question || r.Msg.Names() != tc.refusal || r.Error() != r.Msg.Error() {
			t.Errorf("%s shown=%v: question %q, refusal %q; want %q, %q", tc.q.M().ID(), tc.shown, r.Question.Names(), r.Msg.Names(), tc.question, tc.refusal)
		}
	}
}

// About adds to what's said first, never to the question it came from.
func TestQuestionAbout(t *testing.T) {
	t.Parallel()
	q := Continue().AnsweredBy("--force").About(NoVM("/a"))
	r := q.About(NoVM("/b"))
	q = q.About(NoVM("/c"))
	if len(r.Context()) != 2 || r.Context()[1].Names() != "no_vm dir=/b" || q.Context()[1].Names() != "no_vm dir=/c" {
		t.Errorf("contexts %v and %v", r.Context(), q.Context())
	}
	if q.M().ID() != "continue" || q.Refused(true).Msg.Names() != "no_input answer=--force" {
		t.Errorf("asks %q, refused with %q", q.M().ID(), q.Refused(true).Msg.Names())
	}
}
