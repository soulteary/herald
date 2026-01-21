package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
)

var (
	initOnce sync.Once
	enabled  atomic.Bool

	challengesTotal = newCounterVec(
		"herald_otp_challenges_total",
		"Total number of OTP challenge requests.",
		[]string{"channel", "purpose", "result"},
	)
	sendsTotal = newCounterVec(
		"herald_otp_sends_total",
		"Total number of OTP send attempts.",
		[]string{"channel", "result"},
	)
	sendDuration = newHistogramVec(
		"herald_otp_send_duration_seconds",
		"Duration of OTP send attempts.",
		[]string{"channel", "result"},
		[]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	)
	verifications = newCounterVec(
		"herald_otp_verifications_total",
		"Total number of OTP verification attempts.",
		[]string{"result", "reason"},
	)
	rateLimitHits = newCounterVec(
		"herald_rate_limit_hits_total",
		"Total number of rate limit hits.",
		[]string{"scope"},
	)
)

// Init enables metrics collection.
func Init(enable bool) {
	if !enable {
		return
	}
	initOnce.Do(func() {
		enabled.Store(true)
	})
}

// Enabled returns true when metrics are enabled.
func Enabled() bool {
	return enabled.Load()
}

// Handler returns a Fiber handler to expose metrics.
func Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !enabled.Load() {
			return c.SendStatus(fiber.StatusNotFound)
		}
		c.Set("Content-Type", "text/plain; version=0.0.4")
		return c.SendString(render())
	}
}

// RecordChallenge increments challenge counter.
func RecordChallenge(channel, purpose, result string) {
	if !enabled.Load() {
		return
	}
	challengesTotal.Inc(channel, purpose, result)
}

// RecordSend increments send counter.
func RecordSend(channel, result string) {
	if !enabled.Load() {
		return
	}
	sendsTotal.Inc(channel, result)
}

// ObserveSendDuration records send duration.
func ObserveSendDuration(channel, result string, duration time.Duration) {
	if !enabled.Load() {
		return
	}
	sendDuration.Observe(duration.Seconds(), channel, result)
}

// RecordVerification increments verification counter.
func RecordVerification(result, reason string) {
	if !enabled.Load() {
		return
	}
	verifications.Inc(result, reason)
}

// RecordRateLimitHit increments rate limit counter.
func RecordRateLimitHit(scope string) {
	if !enabled.Load() {
		return
	}
	rateLimitHits.Inc(scope)
}

func render() string {
	var builder strings.Builder
	renderCounterVec(&builder, challengesTotal)
	renderCounterVec(&builder, sendsTotal)
	renderHistogramVec(&builder, sendDuration)
	renderCounterVec(&builder, verifications)
	renderCounterVec(&builder, rateLimitHits)
	return builder.String()
}

type counterVec struct {
	name      string
	help      string
	labelKeys []string
	mu        sync.Mutex
	values    map[string]*counterValue
}

type counterValue struct {
	labels []string
	value  uint64
}

func newCounterVec(name, help string, labelKeys []string) *counterVec {
	return &counterVec{
		name:      name,
		help:      help,
		labelKeys: append([]string(nil), labelKeys...),
		values:    make(map[string]*counterValue),
	}
}

func (c *counterVec) Inc(labelValues ...string) {
	if len(labelValues) != len(c.labelKeys) {
		return
	}
	key := strings.Join(labelValues, "|")
	c.mu.Lock()
	defer c.mu.Unlock()
	if current, ok := c.values[key]; ok {
		current.value++
		return
	}
	c.values[key] = &counterValue{
		labels: append([]string(nil), labelValues...),
		value:  1,
	}
}

type histogramVec struct {
	name      string
	help      string
	labelKeys []string
	buckets   []float64
	mu        sync.Mutex
	values    map[string]*histogramValue
}

type histogramValue struct {
	labels       []string
	bucketCounts []uint64
	sum          float64
	count        uint64
}

func newHistogramVec(name, help string, labelKeys []string, buckets []float64) *histogramVec {
	sortedBuckets := append([]float64(nil), buckets...)
	sort.Float64s(sortedBuckets)
	return &histogramVec{
		name:      name,
		help:      help,
		labelKeys: append([]string(nil), labelKeys...),
		buckets:   sortedBuckets,
		values:    make(map[string]*histogramValue),
	}
}

func (h *histogramVec) Observe(value float64, labelValues ...string) {
	if len(labelValues) != len(h.labelKeys) {
		return
	}
	key := strings.Join(labelValues, "|")
	h.mu.Lock()
	defer h.mu.Unlock()
	item, ok := h.values[key]
	if !ok {
		item = &histogramValue{
			labels:       append([]string(nil), labelValues...),
			bucketCounts: make([]uint64, len(h.buckets)),
		}
		h.values[key] = item
	}
	item.count++
	item.sum += value
	for i, bound := range h.buckets {
		if value <= bound {
			item.bucketCounts[i]++
		}
	}
}

func renderCounterVec(builder *strings.Builder, metric *counterVec) {
	builder.WriteString("# HELP ")
	builder.WriteString(metric.name)
	builder.WriteString(" ")
	builder.WriteString(metric.help)
	builder.WriteString("\n# TYPE ")
	builder.WriteString(metric.name)
	builder.WriteString(" counter\n")

	metric.mu.Lock()
	defer metric.mu.Unlock()
	for _, item := range metric.values {
		builder.WriteString(metric.name)
		builder.WriteString(formatLabels(metric.labelKeys, item.labels))
		builder.WriteString(" ")
		fmt.Fprintf(builder, "%d\n", item.value)
	}
}

func renderHistogramVec(builder *strings.Builder, metric *histogramVec) {
	builder.WriteString("# HELP ")
	builder.WriteString(metric.name)
	builder.WriteString(" ")
	builder.WriteString(metric.help)
	builder.WriteString("\n# TYPE ")
	builder.WriteString(metric.name)
	builder.WriteString(" histogram\n")

	metric.mu.Lock()
	defer metric.mu.Unlock()
	for _, item := range metric.values {
		for i, bound := range metric.buckets {
			builder.WriteString(metric.name)
			builder.WriteString("_bucket")
			builder.WriteString(formatLabelsWithLE(metric.labelKeys, item.labels, bound))
			builder.WriteString(" ")
			fmt.Fprintf(builder, "%d\n", item.bucketCounts[i])
		}
		builder.WriteString(metric.name)
		builder.WriteString("_bucket")
		builder.WriteString(formatLabelsWithLE(metric.labelKeys, item.labels, "+Inf"))
		builder.WriteString(" ")
		fmt.Fprintf(builder, "%d\n", item.count)

		builder.WriteString(metric.name)
		builder.WriteString("_sum")
		builder.WriteString(formatLabels(metric.labelKeys, item.labels))
		builder.WriteString(" ")
		fmt.Fprintf(builder, "%f\n", item.sum)

		builder.WriteString(metric.name)
		builder.WriteString("_count")
		builder.WriteString(formatLabels(metric.labelKeys, item.labels))
		builder.WriteString(" ")
		fmt.Fprintf(builder, "%d\n", item.count)
	}
}

func formatLabels(labelKeys, labelValues []string) string {
	if len(labelKeys) == 0 {
		return ""
	}
	parts := make([]string, len(labelKeys))
	for i := range labelKeys {
		parts[i] = fmt.Sprintf(`%s="%s"`, labelKeys[i], escapeLabel(labelValues[i]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatLabelsWithLE(labelKeys, labelValues []string, le any) string {
	parts := make([]string, 0, len(labelKeys)+1)
	for i := range labelKeys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, labelKeys[i], escapeLabel(labelValues[i])))
	}
	switch value := le.(type) {
	case float64:
		parts = append(parts, fmt.Sprintf(`le="%g"`, value))
	default:
		parts = append(parts, fmt.Sprintf(`le="%v"`, value))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}
