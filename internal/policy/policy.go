// Пакет policy управляет конфигурацией для каждого потребителя: какие типы
// персональных данных идентифицируются и маскируются, разрешено ли
// восстановление (демаскирование), и режим маскирования.
package policy

import (
	"errors"
	"sync"

	"github.com/duck-driven-llm-proxy-service/internal/masking"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// Policy описывает, как обрабатывается одна система-потребитель.
type Policy struct {
	// Enabled управляет тем, может ли потребитель вообще обращаться к модулю.
	Enabled bool
	// Types перечисляет типы персональных данных для идентификации и
	// маскирования для этого потребителя. Пустое значение означает
	// «все поддерживаемые типы».
	Types []pii.Type
	// RestoreAllowed управляет тем, разрешено ли демаскирование для этого
	// потребителя.
	RestoreAllowed bool
	// Mode — стиль маскирования (token или mask).
	Mode masking.Mode
}

// Default возвращает разрешающую политику для потребителя.
func Default() Policy {
	return Policy{
		Enabled:        true,
		Types:          nil, // все
		RestoreAllowed: true,
		Mode:           masking.ModeToken,
	}
}

// Manager хранит набор политик потребителей и разрешает их по идентификатору
// потребителя.
type Manager struct {
	mu        sync.RWMutex
	policies  map[string]Policy
	allowlist map[string]bool
}

// NewManager создаёт пустой менеджер политик.
func NewManager() *Manager {
	return &Manager{
		policies:  make(map[string]Policy),
		allowlist: make(map[string]bool),
	}
}

// SetPolicy регистрирует или заменяет политику для потребителя.
func (m *Manager) SetPolicy(consumer string, p Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[consumer] = p
}

// SetAllowlist регистрирует набор потребителей, которым разрешено обращаться
// к модулю. Потребители вне списка отклоняются независимо от политики.
func (m *Manager) SetAllowlist(consumers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowlist = make(map[string]bool, len(consumers))
	for _, c := range consumers {
		m.allowlist[c] = true
	}
}

// Allowed сообщает, может ли потребитель обращаться к модулю.
func (m *Manager) Allowed(consumer string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.allowlist) > 0 && !m.allowlist[consumer] {
		return false
	}
	p, ok := m.policies[consumer]
	if !ok {
		return true
	}
	return p.Enabled
}

// For возвращает действующую политику для потребителя.
func (m *Manager) For(consumer string) Policy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.policies[consumer]; ok {
		return p
	}
	return Default()
}

// FilterTypes возвращает типы, которые следует маскировать для потребителя.
// Пустой список типов в политике означает все поддерживаемые типы.
func (p Policy) FilterTypes() map[pii.Type]bool {
	if len(p.Types) == 0 {
		out := make(map[pii.Type]bool, len(pii.AllTypes))
		for _, t := range pii.AllTypes {
			out[t] = true
		}
		return out
	}
	out := make(map[pii.Type]bool, len(p.Types))
	for _, t := range p.Types {
		out[t] = true
	}
	return out
}

// Validate проверяет политику на согласованность.
func (p Policy) Validate() error {
	if p.Mode == masking.ModeMask && p.RestoreAllowed {
		return errors.New("policy: mask mode is not reversible; restore must be disabled")
	}
	for _, t := range p.Types {
		if !t.Valid() {
			return errors.New("policy: unknown type " + string(t))
		}
	}
	return nil
}