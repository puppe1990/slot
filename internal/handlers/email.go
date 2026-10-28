package handlers

import (
	"bytes"
	"html/template"
	"strings"
)

// Calm Conviction on email: table layout and inline styles only, because Gmail
// strips <style> and ignores web fonts. The palette matches the app, so the
// confirmation looks like the page the guest just used.

var emailTones = map[string]struct{ BadgeBackground, BadgeColor, CTABackground string }{
	"pine":  {"#C9EAD7", "#1B382B", "#1B382B"},
	"slate": {"#E4E2E1", "#2C2C2C", "#1B382B"},
	"clay":  {"#FFDBD0", "#C26343", "#C26343"},
}

type emailView struct {
	Lang            string
	Eyebrow         string
	Heading         string
	Tone            string
	Intro           string
	When            string
	Range           string
	Zone            string
	ReasonLabel     string
	Reason          string
	ShowGuest       bool
	GuestName       string
	GuestEmail      string
	GuestWhatsApp   string
	GuestNotes      string
	GuestLabel      string
	EmailLabel      string
	WhatsAppLabel   string
	MessageLabel    string
	HostName        string
	HostEmail       string
	HostLabel       string
	CTAURL          string
	CTALabel        string
	ManageURL       string
	ManageLabel     string
	Footer          string
	FooterNote      string
	BadgeBackground string
	BadgeColor      string
	CTABackground   string
}

var emailTemplate = template.Must(template.New("email").Parse(`<!doctype html>
<html lang="{{ .Lang }}">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{ .Heading }}</title>
</head>
<body style="margin:0;padding:0;background:#FAF9F6;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#FAF9F6;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;background:#F4F3EF;border:1px solid #E5E3DC;border-radius:24px;">
          <tr>
            <td style="padding:28px 28px 0 28px;">
              <span style="font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:20px;font-weight:600;letter-spacing:-0.02em;color:#1B382B;">Slot</span>
              <span style="display:inline-block;width:6px;height:6px;border-radius:999px;background:#ADCEBC;margin-left:4px;vertical-align:middle;"></span>
              <div style="margin-top:8px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#64625D;">{{ .Eyebrow }}</div>
            </td>
          </tr>
          <tr>
            <td style="padding:22px 28px 0 28px;">
              <span style="display:inline-block;background:{{ .BadgeBackground }};color:{{ .BadgeColor }};border-radius:999px;padding:6px 12px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;">{{ .Heading }}</span>
              <p style="margin:14px 0 0 0;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:16px;line-height:1.55;color:#1A1A1A;">{{ .Intro }}</p>
            </td>
          </tr>
          <tr>
            <td style="padding:20px 28px 0 28px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#FFFFFF;border:1px solid #E5E3DC;border-radius:16px;">
                <tr>
                  <td style="padding:16px 18px;">
                    <div style="font-family:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;font-size:15px;color:#1A1A1A;">{{ .When }}</div>
                    <div style="margin-top:4px;font-family:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;color:#64625D;">{{ .Range }} · {{ .Zone }}</div>
                    {{ if .Reason }}<div style="margin-top:10px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:13px;color:#C26343;">{{ .ReasonLabel }}: {{ .Reason }}</div>{{ end }}
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          {{ if .ShowGuest }}
          <tr>
            <td style="padding:16px 28px 0 28px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#FFFFFF;border:1px solid #E5E3DC;border-radius:16px;">
                <tr>
                  <td style="padding:16px 18px;">
                    <div style="font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:11px;font-weight:600;letter-spacing:0.12em;text-transform:uppercase;color:#64625D;">{{ .GuestLabel }}</div>
                    <div style="margin-top:8px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:15px;color:#1A1A1A;">{{ .GuestName }}</div>
                    <div style="margin-top:4px;font-family:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;color:#64625D;">{{ .EmailLabel }}: {{ .GuestEmail }}</div>
                    {{ if .GuestWhatsApp }}<div style="margin-top:2px;font-family:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;color:#64625D;">{{ .WhatsAppLabel }}: {{ .GuestWhatsApp }}</div>{{ end }}
                    {{ if .GuestNotes }}<div style="margin-top:12px;padding-top:10px;border-top:1px solid #E5E3DC;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:11px;font-weight:600;letter-spacing:0.12em;text-transform:uppercase;color:#64625D;">{{ .MessageLabel }}</div><div style="margin-top:4px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:14px;line-height:1.55;color:#1A1A1A;white-space:pre-wrap;">{{ .GuestNotes }}</div>{{ end }}
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          {{ end }}
          {{ if .HostEmail }}
          <tr>
            <td style="padding:16px 28px 0 28px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#FFFFFF;border:1px solid #E5E3DC;border-radius:16px;">
                <tr>
                  <td style="padding:16px 18px;">
                    <div style="font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:11px;font-weight:600;letter-spacing:0.12em;text-transform:uppercase;color:#64625D;">{{ .HostLabel }}</div>
                    <div style="margin-top:8px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:15px;color:#1A1A1A;">{{ .HostName }}</div>
                    <div style="margin-top:4px;font-family:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;color:#64625D;"><a href="mailto:{{ .HostEmail }}" style="color:#1B382B;text-decoration:underline;">{{ .HostEmail }}</a></div>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          {{ end }}
          {{ if .CTAURL }}
          <tr>
            <td align="center" style="padding:24px 28px 0 28px;">
              <a href="{{ .CTAURL }}" style="display:inline-block;background:{{ .CTABackground }};color:#FFFFFF;text-decoration:none;padding:14px 26px;border-radius:12px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:12px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;">{{ .CTALabel }}</a>
            </td>
          </tr>
          {{ end }}
          {{ if .ManageURL }}
          <tr>
            <td align="center" style="padding:14px 28px 0 28px;">
              <a href="{{ .ManageURL }}" style="color:#2C2C2C;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:13px;text-decoration:underline;">{{ .ManageLabel }}</a>
            </td>
          </tr>
          {{ end }}
          <tr>
            <td style="padding:26px 28px 28px 28px;">
              <div style="border-top:1px solid #E5E3DC;padding-top:16px;font-family:'Hanken Grotesk','Segoe UI',Helvetica,Arial,sans-serif;font-size:12px;line-height:1.5;color:#64625D;">
                {{ .Footer }}<br />
                <span style="color:#727974;">{{ .FooterNote }}</span>
              </div>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))

func renderEmail(view emailView) string {
	tones, ok := emailTones[view.Tone]
	if !ok {
		tones = emailTones["pine"]
	}
	view.BadgeBackground = tones.BadgeBackground
	view.BadgeColor = tones.BadgeColor
	view.CTABackground = tones.CTABackground
	if view.Lang == "" {
		view.Lang = "en"
	}

	var buffer bytes.Buffer
	if err := emailTemplate.Execute(&buffer, view); err != nil {
		// The template is static and the values are strings; a failure here is
		// a programming error, not a runtime condition.
		return ""
	}
	return strings.TrimSpace(buffer.String())
}
