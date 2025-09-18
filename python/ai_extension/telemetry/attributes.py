"""
OpenTelemetry span attributes for AI Gateway extensions.

This module defines custom span attributes that provide observability into
AI Gateway's PromptGuard features and webhook integrations.
"""

from typing import Final

# Webhook-related attributes for both request and response webhooks
AI_WEBHOOK_ENDPOINT: Final = "ai.webhook.endpoint"
"""The address of the invoked Webhook service, facilitating endpoint identification (e.g., 'localhost:1234')."""

AI_WEBHOOK_RESULT: Final = "ai.webhook.result"
"""The decision made by the Webhook based on the content ('modified', 'rejected', or 'passed'). This is a core attribute for understanding key decision points in the request flow."""

AI_WEBHOOK_REJECT_REASON: Final = "ai.webhook.reject_reason"
"""If the request was rejected, this attribute provides the specific reason for rejection, which is crucial for problem diagnosis."""

# Regex filtering attributes for PromptGuard regex functionality
AI_REGEX_ACTION: Final = "ai.regex.action"
"""The user-configured prompt guard action ('mask' or 'reject'), indicating the intended behavior."""

AI_REGEX_RESULT: Final = "ai.regex.result"
"""Indicates the outcome of the regular expression guard. It's 'reject' if the action is 'reject' and the content was indeed rejected; otherwise, it's 'passed'. This helps quickly identify rejected traffic."""

# Content moderation attributes
AI_MODERATION_MODEL: Final = "ai.moderation.model"
""" Indicates the model used for moderation (e.g., `omni-moderation-latest`), distinct from the main LLM model."""

AI_MODERATION_FLAGGED: Final = "ai.moderation.flagged"
"""A boolean value indicating whether the request was rejected by the moderation guardrails due to content moderation (true or false), serving as a direct measure of moderation effectiveness."""
