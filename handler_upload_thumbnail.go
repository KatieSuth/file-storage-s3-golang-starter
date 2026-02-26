package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	//parse the form data
	const maxMemory = 10 * 1024 * 1024 //10MB

	r.ParseMultipartForm(maxMemory)

	//get image data from the form
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	//read image data
	var image []byte
	image, err = io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read image", err)
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

	//determine image extension
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not determine file type", err)
		return
	}

	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Unallowed file type", err)
		return
	}

	typeArr := strings.Split(mediaType, "/")
	ext := typeArr[(len(typeArr) - 1)]

	//save the thumbnail
	imgNameKey := make([]byte, 32)
	rand.Read(imgNameKey)
	imgName := base64.RawURLEncoding.EncodeToString(imgNameKey)

	path := filepath.Join(cfg.assetsRoot, imgName)
	path = path + "." + ext

	diskFile, err := os.Create(path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create the thumbnail file", err)
		return
	}
	defer diskFile.Close()

	sReader := bytes.NewReader(image)

	_, err = io.Copy(diskFile, sReader)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store thumbnail to disk", err)
		return
	}

	//update video metadata with thumbnail URL
	url := "http://localhost:8091/" + path
	video.ThumbnailURL = &url
	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not store thumbnail with video", err)
		return
	}

	//respond with updated metadata
	respondWithJSON(w, http.StatusOK, video)
}
