package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/puppe1990/slot/internal/slotcli"
)

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "slot_bookings_list",
			"description": "List bookings for the Slot consultant. status: upcoming (default), past, cancelled or all.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"upcoming", "past", "cancelled", "all"}},
					"days":   map[string]any{"type": "integer", "minimum": 1, "description": "window in days (default 90)"},
				},
			},
		},
		{
			"name":        "slot_formats_list",
			"description": "List the published session formats with their public booking URL.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "slot_slots_list",
			"description": "List free slots for a format. Returns start_utc (RFC3339) values that slot_booking_create accepts.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"format"},
				"properties": map[string]any{
					"format":   map[string]any{"type": "string", "description": "format slug, e.g. 30min"},
					"days":     map[string]any{"type": "integer", "minimum": 1, "description": "how many days ahead (default 7)"},
					"timezone": map[string]any{"type": "string", "description": "IANA zone for the labels, e.g. America/Sao_Paulo"},
				},
			},
		},
		{
			"name":        "slot_booking_create",
			"description": "Book a slot on behalf of a guest. Fails with 409 when the slot is taken.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"format", "slot", "name", "email"},
				"properties": map[string]any{
					"format":   map[string]any{"type": "string"},
					"slot":     map[string]any{"type": "string", "description": "start_utc returned by slot_slots_list"},
					"name":     map[string]any{"type": "string"},
					"email":    map[string]any{"type": "string"},
					"whatsapp": map[string]any{"type": "string", "description": "optional, with country code"},
					"notes":    map[string]any{"type": "string"},
					"timezone": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "slot_booking_cancel",
			"description": "Cancel a booking by id, optionally recording a reason.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":     map[string]any{"type": "integer"},
					"reason": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "slot_booking_reschedule",
			"description": "Move a booking to another free slot (RFC3339 UTC).",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id", "slot"},
				"properties": map[string]any{
					"id":   map[string]any{"type": "integer"},
					"slot": map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (string, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return "", fmt.Errorf("invalid tools/call params: %w", err)
	}

	switch call.Name {
	case "slot_bookings_list":
		var args struct {
			Status string `json:"status"`
			Days   int    `json:"days"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		bookings, err := s.API.Bookings(ctx, args.Status, args.Days)
		if err != nil {
			return "", err
		}
		return toJSON(bookings)

	case "slot_formats_list":
		formats, err := s.API.Formats(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(formats)

	case "slot_slots_list":
		var args struct {
			Format   string `json:"format"`
			Days     int    `json:"days"`
			Timezone string `json:"timezone"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || strings.TrimSpace(args.Format) == "" {
			return "", fmt.Errorf("format is required")
		}
		slots, err := s.API.Slots(ctx, args.Format, args.Timezone, args.Days)
		if err != nil {
			return "", err
		}
		return toJSON(slots)

	case "slot_booking_create":
		var args slotcli.CreateBookingInput
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		created, err := s.API.CreateBooking(ctx, args)
		if err != nil {
			return "", err
		}
		return toJSON(created)

	case "slot_booking_cancel":
		var args struct {
			ID     int64  `json:"id"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.ID <= 0 {
			return "", fmt.Errorf("id is required")
		}
		booking, err := s.API.Cancel(ctx, args.ID, args.Reason)
		if err != nil {
			return "", err
		}
		return toJSON(booking)

	case "slot_booking_reschedule":
		var args struct {
			ID   int64  `json:"id"`
			Slot string `json:"slot"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.ID <= 0 || args.Slot == "" {
			return "", fmt.Errorf("id and slot are required")
		}
		booking, err := s.API.Reschedule(ctx, args.ID, args.Slot)
		if err != nil {
			return "", err
		}
		return toJSON(booking)

	default:
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
}

func toJSON(payload any) (string, error) {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode result: %w", err)
	}
	return string(encoded), nil
}
