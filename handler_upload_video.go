package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	//set upload limit
	_ = http.MaxBytesReader(w, r.Body, 1073741824) //1GB max

	//parse video ID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	//authenticate the user
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	//get video metadata
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You can't delete this video", err)
		return
	}

	//parse the form data
	const maxMemory = 10 * 1024 * 1024 //10MB
	r.ParseMultipartForm(maxMemory)

	//get video data from the form
	uploadVideo, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer uploadVideo.Close()

	//validate file type
	mediaType := header.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not determine file type", err)
		return
	}

	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Unallowed file type", err)
		return
	}

	//temporarily store the file
	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could create video file", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	//read image data
	var videoFile []byte
	videoFile, err = io.ReadAll(uploadVideo)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read image", err)
		return
	}

	sReader := bytes.NewReader(videoFile)
	_, err = io.Copy(tmpFile, sReader)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store video to disk", err)
		return
	}

	//reset the file pointer
	tmpFile.Seek(0, io.SeekStart)

	//create filename
	videoNameKey := make([]byte, 32)
	rand.Read(videoNameKey)

	aspectRatio, _ := getVideoAspectRatio(tmpFile.Name())
	orientation := "other"

	switch aspectRatio {
	case "9:16":
		orientation = "portrait"
	case "16:9":
		orientation = "landscape"
	}

	videoName := orientation + "/" + base64.RawURLEncoding.EncodeToString(videoNameKey) + ".mp4"

	//save to S3
	emptyContext := context.Background()

	objectInput := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &videoName,
		Body:        tmpFile,
		ContentType: &mimeType,
	}

	_, err = cfg.s3Client.PutObject(emptyContext, &objectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store video to s3", err)
		return
	}

	//update video metadata with video URL
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, videoName)
	video.VideoURL = &url
	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store video file with video", err)
		return
	}

	//respond with updated metadata
	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()

	if err != nil {
		return "", err
	}

	var stream map[string]interface{}

	output := stdout.Bytes()
	err = json.Unmarshal(output, &stream)

	if err != nil {
		return "", err
	}

	streams := stream["streams"].([]interface{})
	aspectRatio := "other"
	for _, stream := range streams {
		str := stream.(map[string]interface{})
		if str["display_aspect_ratio"] == "16:9" || str["display_aspect_ratio"] == "9:16" {
			aspectRatio = fmt.Sprintf("%s", str["display_aspect_ratio"])
		}
		break
	}

	return aspectRatio, nil
}
