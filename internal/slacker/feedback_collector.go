package slacker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// UserFeedback represents user feedback on help quality
type UserFeedback struct {
	UserID     string    `json:"user_id"`
	HelpType   string    `json:"help_type"`
	RequestID  string    `json:"request_id"`
	Rating     int       `json:"rating"` // 1-5 scale
	Comment    string    `json:"comment"`
	Timestamp  time.Time `json:"timestamp"`
	ResponseID string    `json:"response_id"`
	Confidence float64   `json:"confidence"`
	Helpful    bool      `json:"helpful"`
	Resolved   bool      `json:"resolved"`
}

// FeedbackCollector collects and manages user feedback
type FeedbackCollector struct {
	feedback map[string][]*UserFeedback // help_type -> feedback list
	mutex    sync.RWMutex
	metrics  *HelpMetrics
}

// NewFeedbackCollector creates a new feedback collector
func NewFeedbackCollector(metrics *HelpMetrics) *FeedbackCollector {
	return &FeedbackCollector{
		feedback: make(map[string][]*UserFeedback),
		metrics:  metrics,
	}
}

// RecordFeedback stores user feedback for a help response
func (fc *FeedbackCollector) RecordFeedback(feedback *UserFeedback) error {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()

	// Validate feedback
	if feedback.Rating < 1 || feedback.Rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}

	// Set timestamp if not provided
	if feedback.Timestamp.IsZero() {
		feedback.Timestamp = time.Now()
	}

	// Generate request ID if not provided
	if feedback.RequestID == "" {
		feedback.RequestID = fmt.Sprintf("%s_%d", feedback.HelpType, time.Now().Unix())
	}

	// Store feedback
	if fc.feedback[feedback.HelpType] == nil {
		fc.feedback[feedback.HelpType] = make([]*UserFeedback, 0)
	}
	fc.feedback[feedback.HelpType] = append(fc.feedback[feedback.HelpType], feedback)

	// Update metrics
	fc.metrics.RecordUserFeedback(feedback.HelpType, feedback.Rating, feedback.Comment)

	return nil
}

// GetFeedbackSummary returns a summary of feedback for a specific help type
func (fc *FeedbackCollector) GetFeedbackSummary(helpType string) *FeedbackSummary {
	fc.mutex.RLock()
	defer fc.mutex.RUnlock()

	feedbackList, exists := fc.feedback[helpType]
	if !exists || len(feedbackList) == 0 {
		return &FeedbackSummary{
			HelpType:      helpType,
			TotalRating:   0,
			AverageRating: 0,
			Count:         0,
			HelpfulCount:  0,
		}
	}

	var totalRating int
	var helpfulCount int
	var resolvedCount int

	for _, feedback := range feedbackList {
		totalRating += feedback.Rating
		if feedback.Helpful {
			helpfulCount++
		}
		if feedback.Resolved {
			resolvedCount++
		}
	}

	averageRating := float64(totalRating) / float64(len(feedbackList))
	helpfulPercentage := float64(helpfulCount) / float64(len(feedbackList)) * 100
	resolvedPercentage := float64(resolvedCount) / float64(len(feedbackList)) * 100

	return &FeedbackSummary{
		HelpType:           helpType,
		TotalRating:        totalRating,
		AverageRating:      averageRating,
		Count:              len(feedbackList),
		HelpfulCount:       helpfulCount,
		ResolvedCount:      resolvedCount,
		HelpfulPercentage:  helpfulPercentage,
		ResolvedPercentage: resolvedPercentage,
	}
}

// GetOverallSummary returns overall feedback summary across all help types
func (fc *FeedbackCollector) GetOverallSummary() *OverallFeedbackSummary {
	fc.mutex.RLock()
	defer fc.mutex.RUnlock()

	var totalFeedback int
	var totalRating int
	var totalHelpful int
	var totalResolved int
	typeSummaries := make(map[string]*FeedbackSummary)

	for helpType, feedbackList := range fc.feedback {
		if len(feedbackList) == 0 {
			continue
		}

		var typeRating int
		var typeHelpful int
		var typeResolved int

		for _, feedback := range feedbackList {
			totalRating += feedback.Rating
			typeRating += feedback.Rating
			totalFeedback++
			if feedback.Helpful {
				totalHelpful++
				typeHelpful++
			}
			if feedback.Resolved {
				totalResolved++
				typeResolved++
			}
		}

		averageRating := float64(typeRating) / float64(len(feedbackList))
		helpfulPercentage := float64(typeHelpful) / float64(len(feedbackList)) * 100
		resolvedPercentage := float64(typeResolved) / float64(len(feedbackList)) * 100

		typeSummaries[helpType] = &FeedbackSummary{
			HelpType:           helpType,
			TotalRating:        typeRating,
			AverageRating:      averageRating,
			Count:              len(feedbackList),
			HelpfulCount:       typeHelpful,
			ResolvedCount:      typeResolved,
			HelpfulPercentage:  helpfulPercentage,
			ResolvedPercentage: resolvedPercentage,
		}
	}

	var overallAverageRating float64
	var overallHelpfulPercentage float64
	var overallResolvedPercentage float64

	if totalFeedback > 0 {
		overallAverageRating = float64(totalRating) / float64(totalFeedback)
		overallHelpfulPercentage = float64(totalHelpful) / float64(totalFeedback) * 100
		overallResolvedPercentage = float64(totalResolved) / float64(totalFeedback) * 100
	}

	return &OverallFeedbackSummary{
		TotalFeedback:             totalFeedback,
		OverallAverageRating:      overallAverageRating,
		OverallHelpfulPercentage:  overallHelpfulPercentage,
		OverallResolvedPercentage: overallResolvedPercentage,
		TypeSummaries:             typeSummaries,
	}
}

// FeedbackSummary represents feedback summary for a specific help type
type FeedbackSummary struct {
	HelpType           string  `json:"help_type"`
	TotalRating        int     `json:"total_rating"`
	AverageRating      float64 `json:"average_rating"`
	Count              int     `json:"count"`
	HelpfulCount       int     `json:"helpful_count"`
	ResolvedCount      int     `json:"resolved_count"`
	HelpfulPercentage  float64 `json:"helpful_percentage"`
	ResolvedPercentage float64 `json:"resolved_percentage"`
}

// OverallFeedbackSummary represents overall feedback across all help types
type OverallFeedbackSummary struct {
	TotalFeedback             int                         `json:"total_feedback"`
	OverallAverageRating      float64                     `json:"overall_average_rating"`
	OverallHelpfulPercentage  float64                     `json:"overall_helpful_percentage"`
	OverallResolvedPercentage float64                     `json:"overall_resolved_percentage"`
	TypeSummaries             map[string]*FeedbackSummary `json:"type_summaries"`
}

// FeedbackCollectorEnhanced wraps HelpIntegrator with feedback collection
type FeedbackCollectorEnhanced struct {
	*HelpIntegrator
	feedbackCollector *FeedbackCollector
}

// NewFeedbackCollectorEnhanced creates an enhanced help integrator with feedback
func NewFeedbackCollectorEnhanced(integrator *HelpIntegrator, metrics *HelpMetrics) *FeedbackCollectorEnhanced {
	return &FeedbackCollectorEnhanced{
		HelpIntegrator:    integrator,
		feedbackCollector: NewFeedbackCollector(metrics),
	}
}

// HandleIntentFailureWithFeedback provides help and collects feedback
func (fce *FeedbackCollectorEnhanced) HandleIntentFailureWithFeedback(ctx context.Context, userID, channelID, message string) (*HelpAnalysis, error) {
	// Get the help analysis
	analysis, err := fce.HelpIntegrator.HandleIntentFailure(ctx, userID, channelID, message)
	if err != nil {
		return nil, err
	}

	// Create feedback response with interactive elements
	_ = fce.createFeedbackResponse(analysis, "intent_failure", userID)

	// In a real implementation, this would send the response with interactive feedback buttons
	// For now, we'll just log the feedback opportunity
	return analysis, nil
}

// createFeedbackResponse creates a help response that includes feedback collection
func (fce *FeedbackCollectorEnhanced) createFeedbackResponse(analysis *HelpAnalysis, helpType, userID string) *HelpResponse {
	helpResponse := fce.HelpIntegrator.CreateHelpResponse(analysis)

	// Add feedback collection to the response
	helpResponse.Text += "\n\n📝 **Was this helpful?**\n" +
		"Please rate this help (1-5) and provide feedback to improve future assistance.\n" +
		"Reply with: `feedback <rating> <comment>` to help us improve!"

	return helpResponse
}

// ProcessFeedback processes user feedback from message
func (fce *FeedbackCollectorEnhanced) ProcessFeedback(userID, message string) error {
	// Parse feedback message format: "feedback <rating> <comment>"
	var rating int
	var comment string

	// Simple parsing - in a real implementation this would be more robust
	if n, err := fmt.Sscanf(message, "feedback %d", &rating); n == 1 && err == nil {
		// Extract comment after rating
		parts := fmt.Sprintf("%d", rating)
		if idx := len(parts) + len("feedback "); idx < len(message) {
			comment = message[idx:]
		}

		feedback := &UserFeedback{
			UserID:    userID,
			HelpType:  "general", // Could be enhanced to track specific help types
			Rating:    rating,
			Comment:   comment,
			Timestamp: time.Now(),
			Helpful:   rating >= 3, // Consider 3+ as helpful
			Resolved:  rating >= 4, // Consider 4+ as resolved
		}

		return fce.feedbackCollector.RecordFeedback(feedback)
	}

	return fmt.Errorf("invalid feedback format. Use: feedback <1-5> <optional comment>")
}

// ExportFeedback exports all feedback data as JSON for analysis
func (fc *FeedbackCollector) ExportFeedback() ([]byte, error) {
	fc.mutex.RLock()
	defer fc.mutex.RUnlock()

	return json.MarshalIndent(fc.feedback, "", "  ")
}

// GetTopRatedHelpTypes returns help types sorted by average rating
func (fc *FeedbackCollector) GetTopRatedHelpTypes(limit int) []*FeedbackSummary {
	fc.mutex.RLock()
	defer fc.mutex.RUnlock()

	var summaries []*FeedbackSummary
	for helpType := range fc.feedback {
		summary := fc.GetFeedbackSummary(helpType)
		summaries = append(summaries, summary)
	}

	// Sort by average rating (highest first)
	for i := 0; i < len(summaries)-1; i++ {
		for j := i + 1; j < len(summaries); j++ {
			if summaries[j].AverageRating > summaries[i].AverageRating {
				summaries[i], summaries[j] = summaries[j], summaries[i]
			}
		}
	}

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries
}
