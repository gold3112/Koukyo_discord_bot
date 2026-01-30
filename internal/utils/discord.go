package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// FormatUserDisplayName formats a user label as "name#id", "name", or "ID:id".
func FormatUserDisplayName(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%s#%s", name, id)
	case name != "":
		return name
	case id != "":
		return fmt.Sprintf("ID:%s", id)
	default:
		return "-"
	}
}

// DecodePictureDataURL converts a base64 image data URL into a discord file.
func DecodePictureDataURL(value string) *discordgo.File {
	if value == "" || !strings.HasPrefix(value, "data:image/") {
		return nil
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return nil
	}
	header := parts[0]
	payload := parts[1]
	if !strings.Contains(header, ";base64") {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil
	}
	ext := "png"
	switch {
	case strings.Contains(header, "image/jpeg"):
		ext = "jpg"
	case strings.Contains(header, "image/webp"):
		ext = "webp"
	}
	filename := "user_picture." + ext
	return &discordgo.File{
		Name:        filename,
		ContentType: strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:"),
		Reader:      bytes.NewReader(data),
	}
}
