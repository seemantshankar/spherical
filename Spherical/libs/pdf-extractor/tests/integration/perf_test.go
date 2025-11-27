package integration

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/spherical/pdf-extractor/internal/domain"
	"github.com/spherical/pdf-extractor/internal/extract"
	"github.com/spherical/pdf-extractor/internal/llm"
	"github.com/spherical/pdf-extractor/internal/pdf"
)

func init() {
	// Load .env file for testing
	_ = godotenv.Load("../../.env")
}

// TestMemoryUsageSequentialProcessing verifies that sequential processing
// doesn't cause excessive memory growth
func TestMemoryUsageSequentialProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Skip if sample PDF doesn't exist
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	// Force GC before starting
	runtime.GC()

	var memStatsBefore runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize components
	converter := pdf.NewConverter()
	defer converter.Cleanup()

	llmClient := llm.NewClient(apiKey, "")
	extractor := extract.NewService(converter, llmClient)

	eventCh := make(chan domain.StreamEvent, 100)

	// Process PDF
	go func() {
		err := extractor.Process(ctx, testPDFPath, eventCh)
		if err != nil {
			t.Errorf("Process failed: %v", err)
		}
		close(eventCh)
	}()

	// Consume events
	pagesProcessed := 0
	for event := range eventCh {
		if event.Type == domain.EventPageComplete {
			pagesProcessed++

			// Check memory after each page
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			// Log memory usage
			t.Logf("Page %d: Alloc=%d MB, TotalAlloc=%d MB, Sys=%d MB",
				pagesProcessed,
				memStats.Alloc/1024/1024,
				memStats.TotalAlloc/1024/1024,
				memStats.Sys/1024/1024)
		}
	}

	// Force GC after processing
	runtime.GC()

	var memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsAfter)

	// Calculate memory growth
	memGrowth := memStatsAfter.Alloc - memStatsBefore.Alloc
	t.Logf("Memory growth: %d MB", memGrowth/1024/1024)
	t.Logf("Pages processed: %d", pagesProcessed)

	// Verify reasonable memory usage (< 500MB for sequential processing)
	const maxMemoryMB = 500
	if memStatsAfter.Alloc/1024/1024 > maxMemoryMB {
		t.Errorf("Memory usage too high: %d MB (max %d MB)",
			memStatsAfter.Alloc/1024/1024, maxMemoryMB)
	}
}

// BenchmarkPDFConversion benchmarks PDF to image conversion
func BenchmarkPDFConversion(b *testing.B) {
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		b.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	converter := pdf.NewConverter()
	defer converter.Cleanup()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := converter.Convert(ctx, testPDFPath, 85)
		if err != nil {
			b.Fatalf("Convert failed: %v", err)
		}
		converter.Cleanup()
	}
}

// TestCleanupMemoryRelease verifies that cleanup releases memory
func TestCleanupMemoryRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cleanup test in short mode")
	}

	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Sample PDF not found at %s", testPDFPath)
	}

	converter := pdf.NewConverter()
	ctx := context.Background()

	// Convert PDF
	images, err := converter.Convert(ctx, testPDFPath, 85)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if len(images) == 0 {
		t.Fatal("No images converted")
	}

	// Verify temp files exist
	for _, img := range images {
		if _, err := os.Stat(img.ImagePath); os.IsNotExist(err) {
			t.Errorf("Temp file doesn't exist: %s", img.ImagePath)
		}
	}

	// Cleanup
	err = converter.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify temp files are removed
	for _, img := range images {
		if _, err := os.Stat(img.ImagePath); !os.IsNotExist(err) {
			t.Errorf("Temp file still exists after cleanup: %s", img.ImagePath)
		}
	}
}
