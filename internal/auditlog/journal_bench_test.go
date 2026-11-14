package auditlog

import (
	"context"
	"fmt"
	"testing"

	audit "github.com/soulteary/audit-kit/v2"
)

// BenchmarkJournalWrite measures durable acceptance of audit records by
// concurrent writers. Each Write returns only after its record's file and
// directory entry are synced, so the result depends on the storage under the
// benchmark's temporary directory. The backend accepts every record, so
// delivery and cleanup run alongside, as they do in service.
//
//	go test -run '^$' -bench BenchmarkJournalWrite -benchtime 2s ./internal/auditlog/
func BenchmarkJournalWrite(b *testing.B) {
	for _, writers := range []int{1, 4, 16, 64} {
		b.Run(fmt.Sprintf("writers=%d", writers), func(b *testing.B) {
			j, err := newJournalStorage(audit.NewNoopStorage(), b.TempDir(), 1<<40, 1000, 2, nil)
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).
				WithChallengeID("ch_benchmark0123456789ab").WithUserID("benchmark-user")
			ctx := context.Background()

			work := make(chan struct{}, b.N)
			for i := 0; i < b.N; i++ {
				work <- struct{}{}
			}
			close(work)
			errs := make(chan error, writers)
			b.ResetTimer()
			for w := 0; w < writers; w++ {
				go func() {
					var failure error
					for range work {
						if err := j.Write(ctx, record); err != nil && failure == nil {
							failure = err
						}
					}
					errs <- failure
				}()
			}
			for w := 0; w < writers; w++ {
				if err := <-errs; err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "records/s")
		})
	}
}
