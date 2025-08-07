package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"go.uber.org/zap"
)

// ActionType defines the types of actions a business rule can perform
type ActionType string

const (
	ActionTypeUpdate  ActionType = "update"
	ActionTypeCreate  ActionType = "create"
	ActionTypeDelete  ActionType = "delete"
	ActionTypeNotify  ActionType = "notify"
	ActionTypeCompute ActionType = "compute"
)

// RuleAction defines what happens when a rule condition is met
type RuleAction struct {
	Type      ActionType         `json:"type"`
	Target    string             `json:"target"`              // collection name for data actions
	Data      *query.FilterValue  `json:"data,omitempty"`      // computed values to persist/send
	Condition *query.QueryFilter `json:"condition,omitempty"` // additional conditional execution
	Template  map[string]any     `json:"template,omitempty"`  // data template for creates/updates
	Webhook   *WebhookConfig     `json:"webhook,omitempty"`   // for notify actions
	Priority  int                `json:"priority,omitempty"`  // execution order
}

// WebhookConfig defines notification endpoints
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // defaults to POST
	Headers map[string]string `json:"headers,omitempty"`
	Timeout *time.Duration    `json:"timeout,omitempty"`
}

// BusinessRule combines your query DSL with actions
type BusinessRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Rule conditions using your existing query DSL
	Triggers   []RuleTrigger `json:"triggers"`   // what events/conditions activate this rule
	Conditions *query.Query  `json:"conditions"` // query to evaluate against data
	Actions    []RuleAction  `json:"actions"`    // what to do when conditions match

	// Rule metadata
	Priority   int        `json:"priority"` // higher numbers execute first
	Enabled    bool       `json:"enabled"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	Tags       []string   `json:"tags,omitempty"`

	// Execution tracking
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastFired *time.Time `json:"last_fired,omitempty"`
	FireCount int64      `json:"fire_count"`
}

// RuleTrigger defines when a rule should be evaluated
type RuleTrigger struct {
	Type       TriggerType               `json:"type"`
	Event      base.PersistenceEventType `json:"event,omitempty"`      // for event-driven triggers
	Collection string                    `json:"collection,omitempty"` // target collection
	Schedule   *ScheduleConfig           `json:"schedule,omitempty"`   // for scheduled triggers
}

type TriggerType string

const (
	TriggerTypeEvent     TriggerType = "event"     // react to persistence events
	TriggerTypeScheduled TriggerType = "scheduled" // cron-like scheduling
	TriggerTypeOnDemand  TriggerType = "on_demand" // manual execution
)

// ScheduleConfig defines cron-like scheduling
type ScheduleConfig struct {
	Cron     string        `json:"cron,omitempty"`     // cron expression
	Interval time.Duration `json:"interval,omitempty"` // simple interval
}

// BusinessRuleEngine orchestrates rule execution using your existing components
type BusinessRuleEngine struct {
	persistence    base.Persistence
	queryEngine    *query.QueryEngine
	rules          map[string]*BusinessRule
	subscriptions  map[string]string // rule ID -> subscription ID
	actionExecutor *ActionExecutor
	scheduler      *RuleScheduler
	logger         *zap.Logger
}

// NewBusinessRuleEngine creates a new rule engine
func NewBusinessRuleEngine(
	persistence base.Persistence,
	queryEngine *query.QueryEngine,
	logger *zap.Logger,
) *BusinessRuleEngine {
	if logger == nil {
		logger = zap.NewNop()
	}

	engine := &BusinessRuleEngine{
		persistence:    persistence,
		queryEngine:    queryEngine,
		rules:          make(map[string]*BusinessRule),
		subscriptions:  make(map[string]string),
		actionExecutor: NewActionExecutor(persistence, logger),
		scheduler:      NewRuleScheduler(logger),
		logger:         logger,
	}

	return engine
}

// RegisterRule adds a new business rule and sets up its triggers
func (bre *BusinessRuleEngine) RegisterRule(rule *BusinessRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}

	// Validate rule structure
	if err := bre.validateRule(rule); err != nil {
		return fmt.Errorf("invalid rule: %w", err)
	}

	// Store the rule
	bre.rules[rule.ID] = rule

	// Set up triggers
	if err := bre.setupTriggers(rule); err != nil {
		delete(bre.rules, rule.ID)
		return fmt.Errorf("failed to setup triggers: %w", err)
	}

	bre.logger.Info("Rule registered",
		zap.String("rule_id", rule.ID),
		zap.String("name", rule.Name),
		zap.Int("triggers", len(rule.Triggers)))

	return nil
}

// UnregisterRule removes a business rule and cleans up its triggers
func (bre *BusinessRuleEngine) UnregisterRule(ruleID string) error {
	rule, exists := bre.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	// Clean up triggers
	bre.cleanupTriggers(rule)

	// Remove from registry
	delete(bre.rules, ruleID)

	bre.logger.Info("Rule unregistered", zap.String("rule_id", ruleID))
	return nil
}

// ExecuteRule manually executes a specific rule against provided data
func (bre *BusinessRuleEngine) ExecuteRule(
	ctx context.Context,
	ruleID string,
	inputData common.Document,
) (*RuleExecutionResult, error) {
	rule, exists := bre.rules[ruleID]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", ruleID)
	}

	if !rule.Enabled {
		return &RuleExecutionResult{
			RuleID: ruleID,
			Status: ExecutionStatusSkipped,
			Reason: "rule is disabled",
		}, nil
	}

	// Check temporal validity
	if !bre.isRuleValidAt(rule, time.Now()) {
		return &RuleExecutionResult{
			RuleID: ruleID,
			Status: ExecutionStatusSkipped,
			Reason: "rule is outside valid time range",
		}, nil
	}

	return bre.executeRuleLogic(ctx, rule, inputData)
}

// executeRuleLogic contains the core rule execution logic
func (bre *BusinessRuleEngine) executeRuleLogic(
	ctx context.Context,
	rule *BusinessRule,
	inputData common.Document,
) (*RuleExecutionResult, error) {
	startTime := time.Now()
	result := &RuleExecutionResult{
		RuleID:    rule.ID,
		StartTime: startTime,
		InputData: inputData,
	}

	// Create a temporary collection from input data for querying
	tempData := []common.Document{inputData}

	// Execute the rule conditions using your query engine
	queryHelper, err := query.NewQueryHelper(rule.Conditions, nil, nil, nil)
	if err != nil {
		result.Status = ExecutionStatusError
		result.Error = fmt.Sprintf("failed to create query helper: %v", err)
		return result, err
	}

	// Filter the data using rule conditions
	matchedData, err := queryHelper.Filter(tempData)
	if err != nil {
		result.Status = ExecutionStatusError
		result.Error = fmt.Sprintf("rule condition evaluation failed: %v", err)
		return result, err
	}

	// If no data matches, rule doesn't fire
	if len(matchedData) == 0 {
		result.Status = ExecutionStatusNoMatch
		result.ExecutionTime = time.Since(startTime)
		return result, nil
	}

	// Execute actions
	actionResults := make([]ActionResult, 0, len(rule.Actions))
	for _, action := range rule.Actions {
		actionResult := bre.actionExecutor.Execute(ctx, &action, matchedData[0])
		actionResults = append(actionResults, actionResult)

		// Stop on first error if action is critical
		if actionResult.Error != nil && action.Priority > 0 {
			result.Status = ExecutionStatusError
			result.Error = fmt.Sprintf("critical action failed: %v", actionResult.Error)
			result.ActionResults = actionResults
			result.ExecutionTime = time.Since(startTime)
			return result, actionResult.Error
		}
	}

	// Update rule statistics
	rule.LastFired = &startTime
	rule.FireCount++

	result.Status = ExecutionStatusSuccess
	result.ActionResults = actionResults
	result.ExecutionTime = time.Since(startTime)
	result.OutputData = matchedData[0]

	bre.logger.Info("Rule executed successfully",
		zap.String("rule_id", rule.ID),
		zap.Duration("execution_time", result.ExecutionTime),
		zap.Int("actions_executed", len(actionResults)))

	return result, nil
}

// Event handler for persistence events
func (bre *BusinessRuleEngine) handlePersistenceEvent(ctx context.Context, event base.PersistenceEvent) error {
	bre.logger.Debug("Processing persistence event",
		zap.String("event_type", string(event.Type)),
		zap.String("collection", *event.Collection))

	// Find rules triggered by this event
	for _, rule := range bre.rules {
		if !rule.Enabled {
			continue
		}

		// Check if rule is triggered by this event
		if bre.shouldRuleFireForEvent(rule, event) {
			// Extract data from event for rule evaluation
			inputData := bre.extractDataFromEvent(event)
			if inputData == nil {
				continue
			}

			// Execute rule asynchronously to avoid blocking event processing
			go func(r *BusinessRule, data common.Document) {
				result, err := bre.executeRuleLogic(ctx, r, data)
				if err != nil {
					bre.logger.Error("Rule execution failed",
						zap.String("rule_id", r.ID),
						zap.Error(err))
				} else {
					bre.logger.Debug("Event-triggered rule executed",
						zap.String("rule_id", r.ID),
						zap.String("status", string(result.Status)))
				}
			}(rule, inputData)
		}
	}

	return nil
}

// Helper functions for rule management
func (bre *BusinessRuleEngine) validateRule(rule *BusinessRule) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name cannot be empty")
	}
	if len(rule.Triggers) == 0 {
		return fmt.Errorf("rule must have at least one trigger")
	}
	if len(rule.Actions) == 0 {
		return fmt.Errorf("rule must have at least one action")
	}
	return nil
}

func (bre *BusinessRuleEngine) setupTriggers(rule *BusinessRule) error {
	for _, trigger := range rule.Triggers {
		switch trigger.Type {
		case TriggerTypeEvent:
			subscriptionID := bre.persistence.RegisterSubscription(base.RegisterSubscriptionOptions{
				Event:       trigger.Event,
				Label:       &rule.Name,
				Description: &rule.Description,
				Callback:    bre.handlePersistenceEvent,
			})
			bre.subscriptions[rule.ID] = subscriptionID

		case TriggerTypeScheduled:
			if trigger.Schedule != nil {
				bre.scheduler.ScheduleRule(rule.ID, trigger.Schedule, func() {
					// For scheduled rules, we might query the database for current data
					// This is a simplified example
					inputData := common.Document{"_trigger": "scheduled"}
					bre.executeRuleLogic(context.Background(), rule, inputData)
				})
			}
		}
	}
	return nil
}

func (bre *BusinessRuleEngine) cleanupTriggers(rule *BusinessRule) {
	// Remove event subscriptions
	if subscriptionID, exists := bre.subscriptions[rule.ID]; exists {
		bre.persistence.UnregisterSubscription(subscriptionID)
		delete(bre.subscriptions, rule.ID)
	}

	// Remove scheduled triggers
	bre.scheduler.UnscheduleRule(rule.ID)
}

func (bre *BusinessRuleEngine) isRuleValidAt(rule *BusinessRule, t time.Time) bool {
	if rule.ValidFrom != nil && t.Before(*rule.ValidFrom) {
		return false
	}
	if rule.ValidUntil != nil && t.After(*rule.ValidUntil) {
		return false
	}
	return true
}

func (bre *BusinessRuleEngine) shouldRuleFireForEvent(rule *BusinessRule, event base.PersistenceEvent) bool {
	for _, trigger := range rule.Triggers {
		if trigger.Type == TriggerTypeEvent && trigger.Event == event.Type {
			if trigger.Collection == "" || (event.Collection != nil && *event.Collection == trigger.Collection) {
				return true
			}
		}
	}
	return false
}

func (bre *BusinessRuleEngine) extractDataFromEvent(event base.PersistenceEvent) common.Document {
	// Extract relevant data from the event for rule evaluation
	// This is a simplified implementation
	if event.Output != nil {
		if doc, ok := event.Output.(common.Document); ok {
			return doc
		}
		if doc, ok := event.Output.(map[string]any); ok {
			return common.Document(doc)
		}
	}
	if event.Input != nil {
		if doc, ok := event.Input.(common.Document); ok {
			return doc
		}
		if doc, ok := event.Input.(map[string]any); ok {
			return common.Document(doc)
		}
	}
	return nil
}

// Execution result types
type ExecutionStatus string

const (
	ExecutionStatusSuccess ExecutionStatus = "success"
	ExecutionStatusNoMatch ExecutionStatus = "no_match"
	ExecutionStatusSkipped ExecutionStatus = "skipped"
	ExecutionStatusError   ExecutionStatus = "error"
)

type RuleExecutionResult struct {
	RuleID        string          `json:"rule_id"`
	Status        ExecutionStatus `json:"status"`
	StartTime     time.Time       `json:"start_time"`
	ExecutionTime time.Duration   `json:"execution_time"`
	InputData     common.Document `json:"input_data"`
	OutputData    common.Document `json:"output_data,omitempty"`
	ActionResults []ActionResult  `json:"action_results,omitempty"`
	Error         string          `json:"error,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

type ActionResult struct {
	ActionType ActionType    `json:"action_type"`
	Target     string        `json:"target"`
	Success    bool          `json:"success"`
	Data       any           `json:"data,omitempty"`
	Error      error         `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// Placeholder types for components that would need implementation
type ActionExecutor struct {
	persistence base.Persistence
	logger      *zap.Logger
}

func NewActionExecutor(persistence base.Persistence, logger *zap.Logger) *ActionExecutor {
	return &ActionExecutor{persistence: persistence, logger: logger}
}

func (ae *ActionExecutor) Execute(ctx context.Context, action *RuleAction, data common.Document) ActionResult {
	// Implementation would handle different action types
	// This is a placeholder
	return ActionResult{
		ActionType: action.Type,
		Target:     action.Target,
		Success:    true,
		Duration:   time.Millisecond * 10,
	}
}

type RuleScheduler struct {
	logger *zap.Logger
}

func NewRuleScheduler(logger *zap.Logger) *RuleScheduler {
	return &RuleScheduler{logger: logger}
}

func (rs *RuleScheduler) ScheduleRule(ruleID string, schedule *ScheduleConfig, callback func()) {
	// Implementation would handle cron scheduling
}

func (rs *RuleScheduler) UnscheduleRule(ruleID string) {
	// Implementation would remove scheduled jobs
}
