package msg

// A Question is a message devvm asks, with what answers it when devvm
// can't ask: a flag or a command typed instead (AnsweredBy), or, for a
// question only the user can answer, the refusal that says what to do
// (RefusedWith). Every prompt of a Reporter takes one, so no question is
// asked without its answer declared. About adds what's said before it.
type Question struct {
	m       M
	by      string
	refusal M
	about   []M
}

// AnsweredBy makes m a question that by answers without asking, a flag
// or a command (--force, devvm size -g), which its refusal names
// (no_input).
func (m M) AnsweredBy(by string) Question { return Question{m: m, by: by} }

// RefusedWith makes m a question no flag answers, whose refusal is r:
// what to do instead, such as run the command in a terminal.
func (m M) RefusedWith(r M) Question { return Question{m: m, refusal: r} }

// About returns q with these said before it, only when it's asked.
func (q Question) About(ms ...M) Question {
	q.about = append(q.about[:len(q.about):len(q.about)], ms...)
	return q
}

// M is what's asked.
func (q Question) M() M { return q.m }

// Context is what's said before q, when it's asked.
func (q Question) Context() []M { return q.about }

// Refused is q's refusal when devvm can't ask it: with --no-input,
// without a terminal, or at the end of the input. shown says the prompt
// was printed, so the refusal doesn't show the question again.
func (q Question) Refused(shown bool) *Refusal {
	if q.refusal.ID() != "" {
		return &Refusal{Msg: q.refusal}
	}
	r := &Refusal{Msg: NoInput(q.by)}
	if !shown {
		r.Question = q.m
	}
	return r
}

// A Refusal is the error a Reporter's prompt returns when it can't ask
// its question (Question.Refused): Msg says why and what answers it, and
// Question, when set, is shown before it. Commands return it as any
// failure; a caller with something to fall back on does that instead.
type Refusal struct {
	Question, Msg M
}

func (r *Refusal) Error() string { return r.Msg.Error() }

// Unwrap is Msg, so a refusal is the message it says.
func (r *Refusal) Unwrap() error { return r.Msg }
