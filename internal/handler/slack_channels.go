package handler

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/slackapp"
)

// SlackResourceRouteValidator is the Slack adapter for the provider-neutral
// team external-resource route API. It only understands Slack's resource
// vocabulary; no Hivy channel is created or configured here.
type SlackResourceRouteValidator struct {
	db                 *gorm.DB
	nango              *nango.Client
	listPublicChannels func(context.Context, string) ([]slackapp.Channel, error)
	listBotChannels    func(context.Context, string) ([]slackapp.Channel, error)
	joinChannel        func(context.Context, string, string) (slackapp.Channel, error)
}

func NewSlackResourceRouteValidator(db *gorm.DB, nangoClient *nango.Client) *SlackResourceRouteValidator {
	h := &SlackResourceRouteValidator{db: db, nango: nangoClient}
	h.listPublicChannels = slackapp.ListPublicChannels
	h.listBotChannels = slackapp.ListBotChannels
	h.joinChannel = slackapp.JoinChannel
	return h
}

type joinSlackChannelsRequest struct {
	AllPublic  bool     `json:"all_public,omitempty"`
	ChannelIDs []string `json:"channel_ids,omitempty"`
}

type joinSlackChannelFailure struct {
	ChannelID string `json:"channel_id"`
	Error     string `json:"error"`
}

type joinSlackChannelsResponse struct {
	Joined        int                       `json:"joined"`
	AlreadyMember int                       `json:"already_member"`
	Failed        int                       `json:"failed"`
	Failures      []joinSlackChannelFailure `json:"failures,omitempty"`
	publicReady   bool                      `json:"-"`
	allReady      bool                      `json:"-"`
}

func (h *SlackResourceRouteValidator) availableChannels(ctx context.Context, botToken string) ([]slackapp.Channel, error) {
	publicChannels, err := h.listPublicChannels(ctx, botToken)
	if err != nil {
		return nil, err
	}
	botChannels, err := h.listBotChannels(ctx, botToken)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]slackapp.Channel, len(publicChannels)+len(botChannels))
	for _, ch := range publicChannels {
		if ch.IsArchived {
			continue
		}
		ch.IsPrivate = false
		byID[ch.ID] = ch
	}
	for _, ch := range botChannels {
		if ch.IsArchived || !ch.IsMember {
			continue
		}
		existing := byID[ch.ID]
		if existing.ID != "" {
			existing.IsMember = true
			existing.Topic = firstNonEmpty(existing.Topic, ch.Topic)
			existing.Purpose = firstNonEmpty(existing.Purpose, ch.Purpose)
			if existing.NumMembers == 0 {
				existing.NumMembers = ch.NumMembers
			}
			byID[ch.ID] = existing
			continue
		}
		if ch.IsPrivate {
			byID[ch.ID] = ch
		}
	}
	out := make([]slackapp.Channel, 0, len(byID))
	for _, ch := range byID {
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (h *SlackResourceRouteValidator) joinRequestedChannels(ctx context.Context, botToken string, req joinSlackChannelsRequest) (joinSlackChannelsResponse, error) {
	channels, err := h.availableChannels(ctx, botToken)
	if err != nil {
		return joinSlackChannelsResponse{}, err
	}
	targets := make([]slackapp.Channel, 0, len(channels))
	if req.AllPublic {
		for _, ch := range channels {
			if !ch.IsPrivate {
				targets = append(targets, ch)
			}
		}
	} else {
		byID := make(map[string]slackapp.Channel, len(channels))
		for _, ch := range channels {
			byID[ch.ID] = ch
		}
		seen := map[string]bool{}
		for _, id := range req.ChannelIDs {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if ch, ok := byID[id]; ok {
				targets = append(targets, ch)
			} else {
				targets = append(targets, slackapp.Channel{ID: id, IsPrivate: true})
			}
		}
	}

	result := joinSlackChannelsResponse{}
	for _, ch := range targets {
		if ch.IsMember {
			result.AlreadyMember++
			if !ch.IsPrivate {
				result.publicReady = true
			}
			continue
		}
		if ch.IsPrivate {
			result.Failed++
			result.Failures = append(result.Failures, joinSlackChannelFailure{
				ChannelID: ch.ID,
				Error:     "private channels must already include Hivy",
			})
			continue
		}
		joined, err := h.joinChannel(ctx, botToken, ch.ID)
		if err != nil {
			result.Failed++
			result.Failures = append(result.Failures, joinSlackChannelFailure{ChannelID: ch.ID, Error: err.Error()})
			continue
		}
		if joined.IsMember || joined.ID != "" {
			result.Joined++
			result.publicReady = true
		} else {
			result.AlreadyMember++
		}
	}
	result.allReady = len(targets) > 0 && result.Failed == 0 && result.Joined+result.AlreadyMember == len(targets)
	return result, nil
}
