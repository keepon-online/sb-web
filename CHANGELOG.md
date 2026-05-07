# Changelog

## [Unreleased] - 2026-05-07
### Features
- Added manual Sing-box config publishing into the existing admin node subscription library.
- Added source tracking for published nodes so repeated publishing updates existing Sing-box nodes instead of creating duplicates.
- Added admin APIs and UI controls to publish, inspect, and unpublish Sing-box-generated subscription nodes.

### Design Rationale
- Uses a publish adapter layer so Sing-box remains the node creation workflow and the existing node/tag subscription system remains the distribution workflow.

### Notes & Caveats
- Published nodes are stored as admin-owned public nodes and should be selected in subscribe files by the generated `singbox` tags.
