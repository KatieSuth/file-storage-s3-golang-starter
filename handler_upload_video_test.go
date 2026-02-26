package main

import (
	"testing"
)

func TestHorizontalAspectRatio(t *testing.T) {
	path := "/home/kili/file-storage-s3-golang-starter/samples/boots-video-horizontal.mp4"
	want := "16:9"
	output, err := getVideoAspectRatio(path)
	if want != output || err != nil {
		t.Errorf("expected horizontal video = %s, %v, want %s", output, err, want)
	}
}

func TestVerticalAspectRatio(t *testing.T) {
	path := "/home/kili/file-storage-s3-golang-starter/samples/boots-video-vertical.mp4"
	want := "9:16"
	output, err := getVideoAspectRatio(path)
	if want != output || err != nil {
		t.Errorf("expected vertical video = %s, %v, want %s", output, err, want)
	}
}
