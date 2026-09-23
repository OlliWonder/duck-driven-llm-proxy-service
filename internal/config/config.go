// Пакет config загружает конфигурацию сервиса из переменных окружения
// и JSON-файла конфигурации.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/masking"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
	"github.com/duck-driven-llm-proxy-service/internal/policy"
)

// Config — конфигурация сервиса верхнего уровня.
type Config struct {
	// ListenAddr — адрес прослушивания HTTP, например ":8080".
	ListenAddr string `json:"listen_addr"`
	// AESKey — 32-байтовый ключ в hex, используемый для шифрования хранимых значений.
	AESKey string `json:"aes_key"`
	// StoreTTL — время жизни записи соответствия.
	StoreTTL Duration `json:"store_ttl"`
	// StoreMaxSize — максимальное число записей соответствий.
	StoreMaxSize int `json:"store_max_size"`
	// MaxRPS ограничивает скорость запросов; 0 отключает ограничение.
	MaxRPS int `json:"max_rps"`
	// NEREndpoint — адрес локального NER sidecar.
	NEREndpoint string `json:"ner_endpoint"`
	NERWorkers  int    `json:"ner_workers"`
	// Allowlist — набор потребителей, которым разрешено обращаться к модулю.
	Allowlist []string `json:"allowlist"`
	// Consumers сопоставляет идентификатор потребителя с его политикой.
	Consumers map[string]ConsumerConfig `json:"consumers"`
}

// Duration оборачивает time.Duration с разбором строки JSON (например, "24h").
type Duration time.Duration

// UnmarshalJSON разбирает строку длительности.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// D возвращает лежащий в основе time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// ConsumerConfig — JSON-форма политики потребителя.
type ConsumerConfig struct {
	Enabled        *bool    `json:"enabled"`
	Types          []string `json:"types"`
	RestoreAllowed *bool    `json:"restore_allowed"`
	Mode           string   `json:"mode"`
}

// Default возвращает разумную конфигурацию по умолчанию.
func Default() Config {
	return Config{
		ListenAddr:   ":8080",
		StoreTTL:     Duration(24 * time.Hour),
		StoreMaxSize: 1_000_000,
		MaxRPS:       0,
		NEREndpoint:  "http://127.0.0.1:8090",
		NERWorkers:   1,
		Allowlist:    nil,
		Consumers:    map[string]ConsumerConfig{},
	}
}

// Load читает конфигурацию из JSON-файла (необязательно) и переопределений
// из переменных окружения.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}
	if v := os.Getenv("PII_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("PII_AES_KEY"); v != "" {
		cfg.AESKey = v
	}
	if v := os.Getenv("PII_STORE_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, err
		}
		cfg.StoreTTL = Duration(d)
	}
	if v := os.Getenv("PII_STORE_MAX_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, err
		}
		cfg.StoreMaxSize = n
	}
	if v := os.Getenv("PII_MAX_RPS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, err
		}
		cfg.MaxRPS = n
	}
	if v := os.Getenv("PII_NER_ENDPOINT"); v != "" {
		cfg.NEREndpoint = v
	}
	if v := os.Getenv("PII_NER_WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, err
		}
		cfg.NERWorkers = n
	}
	if cfg.NERWorkers < 1 || cfg.NERWorkers > 32 {
		return cfg, fmt.Errorf("config: ner_workers must be between 1 and 32")
	}
	return cfg, nil
}

// BuildPolicyManager преобразует конфигурацию в policy.Manager.
func (c Config) BuildPolicyManager() (*policy.Manager, error) {
	m := policy.NewManager()
	m.SetAllowlist(c.Allowlist)
	for name, cc := range c.Consumers {
		p := policy.Default()
		if cc.Enabled != nil {
			p.Enabled = *cc.Enabled
		}
		if cc.RestoreAllowed != nil {
			p.RestoreAllowed = *cc.RestoreAllowed
		}
		switch cc.Mode {
		case "", "token":
			p.Mode = masking.ModeToken
		case "mask":
			p.Mode = masking.ModeMask
		default:
			return nil, errors.New("config: unknown mode " + cc.Mode)
		}
		for _, t := range cc.Types {
			pt := pii.Type(t)
			if !pt.Valid() {
				return nil, errors.New("config: unknown type " + t)
			}
			p.Types = append(p.Types, pt)
		}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		m.SetPolicy(name, p)
	}
	return m, nil
}
