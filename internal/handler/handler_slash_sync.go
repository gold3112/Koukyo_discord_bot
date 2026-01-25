package handler

import (
	"fmt"
	"log"
	"reflect"
	"sort"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) SyncSlashCommands(s *discordgo.Session) error {
	log.Println("Syncing slash commands...")

	remoteCommands, err := s.ApplicationCommands(s.State.User.ID, "")
	if err != nil {
		return fmt.Errorf("could not fetch remote commands: %w", err)
	}
	localCommands := h.registry.GetSlashDefinitions()

	remoteCmdsMap := make(map[string]*discordgo.ApplicationCommand, len(remoteCommands))
	for _, cmd := range remoteCommands {
		remoteCmdsMap[cmd.Name] = cmd
	}

	for _, localCmd := range localCommands {
		remoteCmd, exists := remoteCmdsMap[localCmd.Name]
		if exists {
			if !commandsAreEqual(localCmd, remoteCmd) {
				log.Printf("Updating slash command: /%s", localCmd.Name)
				if _, err := s.ApplicationCommandEdit(s.State.User.ID, "", remoteCmd.ID, localCmd); err != nil {
					log.Printf("Failed to update command /%s: %v", localCmd.Name, err)
				}
			}
			delete(remoteCmdsMap, localCmd.Name)
		} else {
			log.Printf("Creating slash command: /%s", localCmd.Name)
			if _, err := s.ApplicationCommandCreate(s.State.User.ID, "", localCmd); err != nil {
				log.Printf("Failed to create command /%s: %v", localCmd.Name, err)
			}
		}
	}

	for _, remoteCmd := range remoteCmdsMap {
		log.Printf("Deleting outdated slash command: /%s", remoteCmd.Name)
		if err := s.ApplicationCommandDelete(s.State.User.ID, "", remoteCmd.ID); err != nil {
			log.Printf("Failed to delete command /%s: %v", remoteCmd.Name, err)
		}
	}

	log.Println("Slash command sync complete.")
	return nil
}

func (h *Handler) Cleanup(s *discordgo.Session) {
	log.Println("Skipping slash command cleanup on shutdown.")
}

func commandsAreEqual(c1, c2 *discordgo.ApplicationCommand) bool {
	if c1.Name != c2.Name || c1.Description != c2.Description {
		return false
	}
	if len(c1.Options) != len(c2.Options) {
		return false
	}

	opts1 := make([]*discordgo.ApplicationCommandOption, len(c1.Options))
	copy(opts1, c1.Options)
	sort.Slice(opts1, func(i, j int) bool { return opts1[i].Name < opts1[j].Name })

	opts2 := make([]*discordgo.ApplicationCommandOption, len(c2.Options))
	copy(opts2, c2.Options)
	sort.Slice(opts2, func(i, j int) bool { return opts2[i].Name < opts2[j].Name })

	for i := range opts1 {
		if !optionsAreEqual(opts1[i], opts2[i]) {
			return false
		}
	}
	return true
}

func optionsAreEqual(o1, o2 *discordgo.ApplicationCommandOption) bool {
	if o1.Type != o2.Type || o1.Name != o2.Name || o1.Description != o2.Description || o1.Required != o2.Required {
		return false
	}
	if len(o1.Choices) != len(o2.Choices) || len(o1.Options) != len(o2.Options) {
		return false
	}

	// Compare choices
	if len(o1.Choices) > 0 {
		// Sort choices by name for consistent comparison
		choices1 := make([]*discordgo.ApplicationCommandOptionChoice, len(o1.Choices))
		copy(choices1, o1.Choices)
		sort.Slice(choices1, func(i, j int) bool { return choices1[i].Name < choices1[j].Name })

		choices2 := make([]*discordgo.ApplicationCommandOptionChoice, len(o2.Choices))
		copy(choices2, o2.Choices)
		sort.Slice(choices2, func(i, j int) bool { return choices2[i].Name < choices2[j].Name })

		if !reflect.DeepEqual(choices1, choices2) {
			return false
		}
	}

	// Compare sub-options recursively
	if len(o1.Options) > 0 {
		subOpts1 := make([]*discordgo.ApplicationCommandOption, len(o1.Options))
		copy(subOpts1, o1.Options)
		sort.Slice(subOpts1, func(i, j int) bool { return subOpts1[i].Name < subOpts1[j].Name })

		subOpts2 := make([]*discordgo.ApplicationCommandOption, len(o2.Options))
		copy(subOpts2, o2.Options)
		sort.Slice(subOpts2, func(i, j int) bool { return subOpts2[i].Name < subOpts2[j].Name })

		for i := range subOpts1 {
			if !optionsAreEqual(subOpts1[i], subOpts2[i]) {
				return false
			}
		}
	}

	return true
}
