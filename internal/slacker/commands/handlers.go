package commands

// This file exists for backward compatibility.
// All handlers have been moved to specialized files:
// - preferences.go: HandleHelp, HandleThinking, HandleTools, HandleDone, HandleVerbose, HandlePreferences
// - sessions.go: HandleResetSession, HandleAddTool, HandleListTools, HandleRemoveTool
// - approval.go: HandleApprove, HandleReject, HandleShareTool
// - modelaccess.go: HandleModelAccess
// - admin.go: HandleAdminHelp, HandleEscalate, SendMessage, FormatHelpResponse
