package ai

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"
)

// systemPrompt contains the core instructions and framework for tag recommendation.
const systemPrompt = `You are a top-tier, multilingual knowledge management and information architecture expert. Your mission is to recommend a set of highly relevant, structured tags for any given note, creating a system that is **globally consistent yet locally relevant**.

# Primary Directive: Language First
**This is your most important instruction.** Your absolute first step is to identify the language of the 'Note Content'. All subsequent tag generation—**both the tag names themselves and the reasons**—MUST be in that detected language. You are expected to conceptually translate the dimensions of the framework into the target language.

# Core Framework: The 6 Conceptual Dimensions
All tags MUST belong to one of the following six **conceptual dimensions**. The English names below are for your reference; you must translate them into the note's language for the actual tags. The structural format is "#dimension/sub-dimension/...".

- **Area**: Life domains or responsibilities (e.g., work, personal, learning).
- **Topic**: The specific subject matter of the note (e.g., technology, psychology, cooking).
- **Action**: The note's intent or purpose (e.g., project, review, brainstorm).
- **Entity**: Specific proper nouns mentioned (e.g., people, organizations, books, software).
- **Type**: The format or genre of the note itself (e.g., journal, meeting-notes, idea, summary, guide).
- **Status**: The user's subjective state or context (e.g., mood, feeling).

# Instructions
1.  **Core Value Summary**: Begin your analysis by providing one concise sentence summarizing this note's core value and its main associated dimensions. Format this as: SUMMARY: [your summary sentence]
2.  **Deep Analysis**: Thoroughly understand the note's core content, context, and underlying intent.
3.  **Quality over Quantity**: Recommend only the most valuable tags. Aim for 4-7 tags total.
4.  **Avoid Redundancy**: Within the same dimension, choose the most precise tag.
5.  **Reuse and Evolve**:
    - Prioritize reusing tags from the "Existing Tags" list **if they are in the correct language**.
    - **Crucially, create new, specific sub-tags in the note's language** to help the system evolve with precision.
6.  **Formatting**: Keep structural symbols like "#", "/", "[]", "()" in English/ASCII for universal compatibility.

# Output Format & CRUCIAL EXAMPLES
**You must provide the output in two parts:** First, provide your core value summary line starting with "SUMMARY:", then on the next line provide your tag recommendations as a single, continuous line of text in the format '[#dimension/tag](reason)'. Observe how the framework is translated in the examples below.

**Example 1 (Note in English):**
Note Content: "Brainstorming new features for the Q3 mobile app redesign. Key ideas include a dark mode and social login integration."
Your Output: 
SUMMARY: This note captures feature ideation for mobile app improvement, focusing on user experience enhancements and authentication solutions.
[#area/work](concerns a professional project) [#topic/software/mobile-app](about a mobile application) [#action/brainstorm](captures the act of generating ideas) [#type/idea-list](the note is a list of ideas)

**Example 2 (Note in Spanish):**
Note Content: "Resumen del libro 'Cien años de soledad'. Me encantaron los temas del realismo mágico y la saga familiar Buendía."
Your Output: 
SUMMARY: Esta nota documenta la reflexión sobre una obra literaria clásica, destacando elementos narrativos del realismo mágico y dinámicas familiares.
[#área/aprendizaje](relacionado con el estudio y la lectura) [#tipo/resumen](es un resumen de una obra) [#tema/literatura/realismo-mágico](el tema principal es un género literario) [#entidad/libro/cien-años-de-soledad](menciona una obra específica)

**Example 3 (Note in German):**
Note Content: "Anleitung zur Installation von Docker auf Ubuntu 22.04. Wichtige Befehle: apt-get update, apt-get install docker-ce."
Your Output: 
SUMMARY: Diese Notiz bietet technische Anweisungen für die Containerisierung-Software-Installation, mit Fokus auf System-Administration und DevOps-Praktiken.
[#bereich/arbeit](bezieht sich auf eine berufliche oder technische aufgabe) [#typ/anleitung](ist eine schritt-für-schritt-anleitung) [#thema/technologie/docker](das kernthema ist die docker-technologie) [#entität/ubuntu-22-04](nennt ein spezifisches betriebssystem)`

// userMessageTemplate contains only the user data to be analyzed.
const userMessageTemplate = `{{if .ExistingTags}}Existing Tags: {{.ExistingTags}}

{{end}}Note Content:
{{.NoteContent}}`

// TagSuggestionRequest represents a tag suggestion request
type TagSuggestionRequest struct {
	Content      string   // The memo content to analyze
	UserTags     []string // User's frequently used tags (optional)
	ExistingTags []string // Tags already in the memo (optional)
}

// TagSuggestion represents a single tag suggestion with reason
type TagSuggestion struct {
	Tag    string
	Reason string
}

// TagSuggestionResponse represents the response from tag suggestion
type TagSuggestionResponse struct {
	Tags []TagSuggestion
}

// SuggestTags suggests tags for memo content using AI
func (c *Client) SuggestTags(ctx context.Context, req *TagSuggestionRequest) (*TagSuggestionResponse, error) {
	// Validate request
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}

	// Prepare user tags context
	userTagsContext := ""
	if len(req.UserTags) > 0 {
		topTags := req.UserTags
		if len(topTags) > 20 {
			topTags = topTags[:20]
		}
		userTagsContext = strings.Join(topTags, ", ")
	}

	// Create user message with user data only
	userTmpl, err := template.New("userMessage").Parse(userMessageTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user message template: %w", err)
	}

	var userMsgBuf bytes.Buffer
	err = userTmpl.Execute(&userMsgBuf, map[string]string{
		"ExistingTags": userTagsContext,
		"NoteContent":  req.Content,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute user message template: %w", err)
	}

	// Make AI request with separated system and user messages
	chatReq := &ChatRequest{
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsgBuf.String()},
		},
		MaxTokens:   8192,
		Temperature: 0.8,
		Timeout:     15 * time.Second,
	}

	response, err := c.Chat(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI response for tag suggestion: %w", err)
	}

	tags := c.parseTagResponse(response.Content)

	// Validate that we got some meaningful response
	if len(tags) == 0 {
		return nil, fmt.Errorf("AI returned no valid tag suggestions")
	}

	return &TagSuggestionResponse{
		Tags: tags,
	}, nil
}

// parseTagResponse parses AI response for [tag](reason) patterns
func (c *Client) parseTagResponse(responseText string) []TagSuggestion {
	tags := make([]TagSuggestion, 0)

	// Remove SUMMARY line if present
	lines := strings.Split(responseText, "\n")
	filteredText := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "SUMMARY:") {
			filteredText += line + "\n"
		}
	}

	// Match [tag](reason) format using regex across filtered response
	pattern := `\[([^\]]+)\]\(([^)]+)\)`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(filteredText, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			tag := strings.TrimSpace(match[1])
			reason := strings.TrimSpace(match[2])

			// Remove # prefix if AI included it
			tag = strings.TrimPrefix(tag, "#")

			// Clean and validate tag
			if tag != "" && len(tag) <= 100 {
				// Limit reason length
				if len(reason) > 100 {
					reason = reason[:100] + "..."
				}
				tags = append(tags, TagSuggestion{
					Tag:    tag,
					Reason: reason,
				})
			}
		}
	}

	return tags
}
