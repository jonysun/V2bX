package limiter

import (
	"fmt"
	"regexp"

	"github.com/InazumaV/V2bX/api/panel"
)

func (l *Limiter) CheckDomainRule(destination string) (reject bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// have rule
	for i := range l.DomainRules {
		if l.DomainRules[i].MatchString(destination) {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) CheckProtocolRule(protocol string) (reject bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.ProtocolRules {
		if l.ProtocolRules[i] == protocol {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) UpdateRule(rule *panel.Rules) error {
	domains := make([]*regexp.Regexp, len(rule.Regexp))
	for i := range rule.Regexp {
		r, err := regexp.Compile(rule.Regexp[i])
		if err != nil {
			return fmt.Errorf("domain rule %d: %w", i, err)
		}
		domains[i] = r
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.DomainRules = domains
	l.ProtocolRules = append([]string(nil), rule.Protocol...)
	return nil
}
