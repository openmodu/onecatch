package mobile

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/openmodu/onecatch/internal/landiscovery"
	"github.com/openmodu/onecatch/internal/service/worker"
)

type reconnectState struct {
	gate    chan struct{}
	config  worker.Config
	checked time.Time
	err     error
}

// Resolve before dispatch, never retry a mutation whose response may have
// been lost. DNS-SD is only an address hint; TLS pinning remains authoritative.
func (s *Service) reconnectWorker(ctx context.Context, config worker.Config) (worker.Config, error) {
	if config.ServerCertificateSHA256 == "" {
		return config, nil
	}
	value, _ := s.reconnects.LoadOrStore(config.ID, &reconnectState{gate: make(chan struct{}, 1)})
	state := value.(*reconnectState)
	select {
	case state.gate <- struct{}{}:
	case <-ctx.Done():
		return worker.Config{}, ctx.Err()
	}
	defer func() { <-state.gate }()
	ctx, cancelReconnect := context.WithTimeout(ctx, 8*time.Second)
	defer cancelReconnect()
	// Re-read after waiting: another request may have relocated or removed it.
	config, err := s.registry.Get(ctx, config.ID)
	if err != nil {
		return worker.Config{}, err
	}
	if !config.Enabled {
		return worker.Config{}, errors.New("worker is disabled")
	}
	if config.ServerCertificateSHA256 == "" {
		return config, nil
	}
	if state.config == config && time.Since(state.checked) < 2*time.Second {
		if state.err != nil {
			return worker.Config{}, state.err
		}
		return config, nil
	}
	probe := func(parent context.Context, candidate worker.Config) error {
		probeCtx, cancel := context.WithTimeout(parent, 1500*time.Millisecond)
		defer cancel()
		health, err := s.client.Health(probeCtx, candidate)
		if err != nil {
			return err
		}
		if health.WorkerID != config.ID {
			return errors.New("discovered worker identity does not match pairing")
		}
		return nil
	}
	originalErr := probe(ctx, config)
	if originalErr == nil {
		state.config, state.checked, state.err = config, time.Now(), nil
		return config, nil
	}
	if err := ctx.Err(); err != nil {
		return worker.Config{}, err
	}
	// Authorization and application errors won't be repaired by a new IP.
	var remote worker.RemoteError
	if errors.As(originalErr, &remote) && remote.Code != "worker_unavailable" {
		return worker.Config{}, originalErr
	}
	resolve := s.resolveWorker
	if resolve == nil {
		resolve = landiscovery.Resolve
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	addresses, resolveErr := resolve(discoveryCtx, config.ServerCertificateSHA256)
	cancel()
	if resolveErr == nil {
		// Probe candidates together so a dead IPv6/VPN route cannot consume
		// the whole retry budget before a working Wi-Fi address is tried.
		candidateCtx, cancelCandidates := context.WithCancel(ctx)
		defer cancelCandidates()
		results := make(chan string, 16)
		pending := 0
		seen := map[string]bool{config.BaseURL: true}
		for _, address := range addresses {
			if len(seen) > 16 {
				break
			}
			if seen[address] {
				continue
			}
			seen[address] = true
			parsed, err := url.Parse(address)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
				continue
			}
			candidate := config
			candidate.BaseURL = address
			pending++
			go func() {
				if err := probe(candidateCtx, candidate); err != nil {
					results <- ""
				} else {
					results <- candidate.BaseURL
				}
			}()
		}
		for range pending {
			var address string
			select {
			case address = <-results:
			case <-ctx.Done():
				return worker.Config{}, ctx.Err()
			}
			if address == "" {
				continue
			}
			cancelCandidates()
			updated, err := s.registry.Relocate(ctx, config, address)
			if err != nil {
				return worker.Config{}, err
			}
			state.config, state.checked, state.err = updated, time.Now(), nil
			return updated, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return worker.Config{}, err
	}
	err = worker.RemoteError{Code: "worker_unavailable", Message: "无法连接已配对的电脑。请确认电脑端已开启手机连接、手机与电脑在同一局域网，并允许本地网络访问；网络禁止设备发现时，可使用新地址重新配对。"}
	state.config, state.checked, state.err = config, time.Now(), err
	return worker.Config{}, err
}
