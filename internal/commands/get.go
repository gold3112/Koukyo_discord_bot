package commands

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type GetCommand struct{}

func NewGetCommand() *GetCommand {
	return &GetCommand{}
}

func (c *GetCommand) Name() string {
	return "get"
}

func (c *GetCommand) Description() string {
	return "画像やデータを取得します。"
}

func (c *GetCommand) ExecuteText(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, "このコマンドはスラッシュコマンドで利用してください。")
	return err
}

func (c *GetCommand) ExecuteSlash(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	GetCommandHandler(s, i)
	return nil
}

func (c *GetCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "get",
		Description: "画像やデータを取得します。",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "coords",
				Description: "タイル座標 (例: 1818-806)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "region",
				Description: "Region名 (例: Tokyo#1)",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "fullsize",
				Description: "フルサイズ画像取得用パラメータ",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "region_full",
				Description: "Region全域画像取得用パラメータ",
				Required:    false,
			},
		},
	}
}

type Region struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	MatrixX int     `json:"matrix_x"`
	MatrixY int     `json:"matrix_y"`
}

type RegionDB map[string]Region

func loadRegionDB(path string) (RegionDB, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	var db RegionDB
	if err := json.Unmarshal(bytes, &db); err != nil {
		return nil, err
	}
	return db, nil
}

func GetCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var (
		coords     string
		region     string
		fullsize   string
		regionFull string
	)
	for _, opt := range options {
		switch opt.Name {
		case "coords":
			coords = opt.StringValue()
		case "region":
			region = opt.StringValue()
		case "fullsize":
			fullsize = opt.StringValue()
		case "region_full":
			regionFull = opt.StringValue()
		}
	}

	if regionFull != "" {
		// 4x4タイル結合・PNG返却（API/画像生成部分で実装）
		respondGet(s, i, "Region全域画像取得: 4x4タイル結合（Go版はAPI/画像生成部分を実装してください）")
		return
	}

	if fullsize != "" {
		parts := strings.Split(fullsize, "-")
		if len(parts) == 6 {
			respondGet(s, i, "フルサイズ画像取得: 左上+サイズ形式（Go版はAPI/画像生成部分を実装してください）")
			return
		} else if len(parts) == 8 {
			respondGet(s, i, "フルサイズ画像取得: 左上+右下形式（Go版はAPI/画像生成部分を実装してください）")
			return
		} else {
			respondGet(s, i, "❌ fullsize形式が正しくありません。6要素または8要素で指定してください。")
			return
		}
	}

	if region != "" {
		db, err := loadRegionDB("data/region_database.json")
		if err != nil {
			respondGet(s, i, "Regionデータベースの読み込みに失敗しました。")
			return
		}
		reg, ok := db[region]
		if !ok {
			respondGet(s, i, "❌ Regionが見つかりません。例: Tokyo#1, Osaka#1 など")
			return
		}
		respondGet(s, i, fmt.Sprintf("Region指定: %s の画像取得（Go版はAPI/画像生成部分を実装してください）", reg.Name))
		return
	}

	if coords != "" {
		parts := strings.Split(coords, "-")
		if len(parts) != 2 {
			respondGet(s, i, "❌ 座標形式が正しくありません: TlX-TlY 例: 1818-806")
			return
		}
		tileX, errX := strconv.Atoi(parts[0])
		tileY, errY := strconv.Atoi(parts[1])
		if errX != nil || errY != nil {
			respondGet(s, i, "❌ 座標値が不正です。整数で指定してください。")
			return
		}
		if tileX < 0 || tileX >= 2048 || tileY < 0 || tileY >= 2048 {
			respondGet(s, i, fmt.Sprintf("❌ タイル座標が範囲外です: %d-%d 有効範囲: 0～2047", tileX, tileY))
			return
		}
		respondGet(s, i, fmt.Sprintf("タイル画像取得: %d-%d（Go版はAPI/画像生成部分を実装してください）", tileX, tileY))
		return
	}

	respondGet(s, i, "❌ 座標またはRegion名を指定してください。coords, region, fullsize, region_full のいずれかを指定")
}

func respondGet(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
}
