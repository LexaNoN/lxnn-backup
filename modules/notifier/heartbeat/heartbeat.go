package heartbeat

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nixys/nxs-backup/modules/logger"
)

// Trigger defines when the heartbeat has to be pinged
const (
	TriggerAlways  = "always"
	TriggerSuccess = "success"
	TriggerFailure = "failure"
)

const (
	pingRetries  = 3
	pingRetryGap = 5 * time.Second
	maxMsgLen    = 200
)

// Opts contains heartbeat options
type Opts struct {
	Name              string
	URL               string
	FailureURL        string
	StartURL          string
	Trigger           string
	Method            string
	Jobs              []string
	ExtraHeaders      map[string]string
	InsecureTLS       bool
	StatusParams      bool
	ConnectionTimeout time.Duration
}

// Heartbeat pings the external monitoring service to tell it the backup is alive.
// Such services (e.g. push monitors of the Uptime Kuma, healthchecks.io checks)
// raise an alert if the ping doesn't come in the expected period of time.
type Heartbeat struct {
	opts Opts
	hc   *http.Client
}

// Status is the result of the backup run the heartbeat reports about
type Status struct {
	Ok       bool
	Message  string
	Duration time.Duration
}

func Init(opts Opts) (*Heartbeat, error) {
	if opts.URL == "" {
		return nil, fmt.Errorf("Heartbeat `%s` init fail. Parameter `url` is not specified. ", opts.Name)
	}

	for _, u := range []string{opts.URL, opts.FailureURL, opts.StartURL} {
		if u == "" {
			continue
		}
		if _, err := url.ParseRequestURI(u); err != nil {
			return nil, fmt.Errorf("Heartbeat `%s` init fail. Failed to parse url \"%s\". %v ", opts.Name, u, err)
		}
	}

	switch opts.Trigger {
	case TriggerAlways, TriggerSuccess, TriggerFailure:
	default:
		return nil, fmt.Errorf("Heartbeat `%s` init fail. Unknown trigger `%s`. Available triggers: 'always', 'success', 'failure'. ",
			opts.Name, opts.Trigger)
	}

	opts.Method = strings.ToUpper(opts.Method)
	switch opts.Method {
	case http.MethodGet, http.MethodPost, http.MethodHead:
	default:
		return nil, fmt.Errorf("Heartbeat `%s` init fail. Unsupported method `%s`. Available methods: 'GET', 'POST', 'HEAD'. ",
			opts.Name, opts.Method)
	}

	if opts.Name == "" {
		opts.Name = opts.URL
	}

	d := &net.Dialer{
		Timeout: opts.ConnectionTimeout * time.Second,
	}

	return &Heartbeat{
		opts: opts,
		hc: &http.Client{
			Timeout: opts.ConnectionTimeout * time.Second,
			Transport: &http.Transport{
				DialContext: d.DialContext,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: opts.InsecureTLS,
				},
			},
		},
	}, nil
}

func (h *Heartbeat) Name() string {
	return h.opts.Name
}

// JobsFilter returns the list of the jobs the heartbeat is bound to.
// An empty list means the heartbeat reports about the whole run.
func (h *Heartbeat) JobsFilter() []string {
	return h.opts.Jobs
}

// Start pings the url that marks the beginning of the backup run
func (h *Heartbeat) Start(logCh chan logger.LogRecord) {
	if h.opts.StartURL == "" {
		return
	}
	h.ping(logCh, h.opts.StartURL, "", nil)
}

// Ping notifies the monitoring service about the finished backup run
func (h *Heartbeat) Ping(logCh chan logger.LogRecord, st Status) {
	u := h.opts.URL

	if !st.Ok && h.opts.FailureURL != "" {
		// an explicitly configured failure url is always used to report a failed run
		u = h.opts.FailureURL
	} else {
		switch h.opts.Trigger {
		case TriggerSuccess:
			if !st.Ok {
				logCh <- logger.Log("", "").Debugf("Heartbeat `%s` is not pinged: the run finished with errors.", h.opts.Name)
				return
			}
		case TriggerFailure:
			if st.Ok {
				logCh <- logger.Log("", "").Debugf("Heartbeat `%s` is not pinged: the run finished successfully.", h.opts.Name)
				return
			}
		}
	}

	h.ping(logCh, u, st.Message, &st)
}

func (h *Heartbeat) ping(logCh chan logger.LogRecord, rawURL, body string, st *Status) {
	u, err := h.buildURL(rawURL, st)
	if err != nil {
		logCh <- logger.Log("", "").Errorf("Failed to build heartbeat `%s` url: %v", h.opts.Name, err)
		return
	}

	for i := 0; i < pingRetries; i++ {
		if i > 0 {
			time.Sleep(pingRetryGap)
		}
		if err = h.do(u, body); err == nil {
			logCh <- logger.Log("", "").Debugf("Heartbeat `%s` pinged.", h.opts.Name)
			return
		}
	}

	logCh <- logger.Log("", "").Errorf("Failed to ping heartbeat `%s`: %v", h.opts.Name, err)
}

func (h *Heartbeat) do(u, body string) error {
	var reqBody io.Reader
	if h.opts.Method == http.MethodPost && body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(h.opts.Method, u, reqBody)
	if err != nil {
		return fmt.Errorf("Can't create heartbeat request: %w ", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	for k, v := range h.opts.ExtraHeaders {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := h.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Unexpected HTTP response code: %d, body: %s ", resp.StatusCode, string(respBody))
	}

	return nil
}

// buildURL adds the run status to the query params. Params that are already
// present in the configured url are kept untouched.
func (h *Heartbeat) buildURL(rawURL string, st *Status) (string, error) {
	if !h.opts.StatusParams || st == nil {
		return rawURL, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	if !q.Has("status") {
		s := "down"
		if st.Ok {
			s = "up"
		}
		q.Set("status", s)
	}
	if !q.Has("msg") {
		q.Set("msg", truncate(st.Message, maxMsgLen))
	}
	if !q.Has("ping") {
		q.Set("ping", strconv.FormatInt(st.Duration.Milliseconds(), 10))
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func truncate(s string, limit int) string {
	rs := []rune(s)
	if len(rs) <= limit {
		return s
	}
	return string(rs[:limit]) + "..."
}
