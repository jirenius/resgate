package service

import "fmt"

type gcState byte

const (
	gcStateStop gcState = iota
	gcStateRoot
	gcStateNone
	gcStateDelete
	gcStateKeep
)

type subRef struct {
	sub      *Subscription
	indirect int
	state    gcState
}

type traverseCallback func(sub *Subscription, state gcState) gcState

func (c *wsConn) tryDelete(s *Subscription) {
	if s.direct > 0 {
		return
	}

	refs := make(map[string]*subRef, len(s.refs)+1)
	rr := &subRef{
		sub:      s,
		indirect: s.indirect,
		state:    gcStateNone,
	}
	refs[s.RID()] = rr

	// Count down indirect references
	c.traverse(s, gcStateRoot, func(s *Subscription, state gcState) gcState {
		if state == gcStateRoot {
			return gcStateNone
		}

		if r, ok := refs[s.RID()]; ok {
			r.indirect--
			return gcStateStop
		}
		refs[s.RID()] = &subRef{
			sub:      s,
			indirect: s.indirect - 1,
			state:    gcStateNone,
		}
		return gcStateNone
	})

	// Quick exit if root reference is not to be deleted
	if rr.indirect > 0 {
		if debug {
			c.Logf("TryDelete %s - Not deleting where indirect = %d", s.RID(), rr.indirect)
		}
		return
	}

	// Mark for deletion
	c.traverse(s, gcStateDelete, func(s *Subscription, state gcState) gcState {
		r := refs[s.RID()]

		if r.state == gcStateKeep {
			return gcStateStop
		}

		if r.indirect > 0 || state == gcStateKeep {
			return gcStateKeep
		}

		if r.state != gcStateNone {
			return gcStateStop
		}

		r.state = gcStateDelete
		return gcStateDelete
	})

	for rid, ref := range refs {
		if ref.state == gcStateDelete {
			ref.sub.Dispose()
			delete(c.subs, rid)
		}
	}

	if debug {
		str := ""
		hasDirect := false
		for rid, sub := range c.subs {
			if sub.direct > 0 {
				hasDirect = true
			}
			str += fmt.Sprintf("\n    %6d %6d - %s", sub.direct, sub.indirect, rid)
		}
		if str == "" {
			str = "\n    No Subscriptions"
			hasDirect = true
		}
		c.Logf("After Unsubscribe: %s", str)

		if !hasDirect {
			panic("No direct subscriptions found!")
		}

	}
}

func (c *wsConn) traverse(s *Subscription, state gcState, cb traverseCallback) {
	if s.direct > 0 {
		return
	}

	state = cb(s, state)
	if state == gcStateStop {
		return
	}

	for _, ref := range s.refs {
		c.traverse(ref.sub, state, cb)
	}
}
