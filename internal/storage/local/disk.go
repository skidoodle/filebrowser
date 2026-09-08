package local

import (
	"context"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/skidoodle/filebrowser/internal/storage"
)

// diskUsage reports volume usage via gopsutil, which works across
// platforms including Windows.
func diskUsage(ctx context.Context, root string) (storage.Usage, error) {
	st, err := disk.UsageWithContext(ctx, root)
	if err != nil {
		return storage.Usage{}, err
	}
	return storage.Usage{Used: st.Used, Total: st.Total}, nil
}
