package worker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var Bitrates = []int{64, 128, 320}

type TranscodeResult struct {
	Bitrate   int
	OutputKey string
	LocalPath string
	SizeBytes int64
}

func Transcode(inputPath string, trackID string, bitrate int, outputDir string) (*TranscodeResult, error) {
	outFile := filepath.Join(outputDir, fmt.Sprintf("%d.ogg", bitrate))
	s3Key := fmt.Sprintf("tracks/%s/%d.ogg", trackID, bitrate)

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
		"-c:a", "libvorbis",
		"-b:a", fmt.Sprintf("%dk", bitrate),
		"-vn",
		"-f", "ogg",
		outFile,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg %dkbps: %w", bitrate, err)
	}

	info, err := os.Stat(outFile)
	if err != nil {
		return nil, fmt.Errorf("stat output: %w", err)
	}

	return &TranscodeResult{
		Bitrate:   bitrate,
		OutputKey: s3Key,
		LocalPath: outFile,
		SizeBytes: info.Size(),
	}, nil
}

func ProbeDuration(inputPath string) (int, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return int(seconds * 1000), nil
}
