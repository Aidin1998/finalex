# Pincex Admin Dashboard – KYC/AML Compliance UI Structure

## Overview
A React (or similar) admin dashboard for compliance officers to:
- Review KYC cases and documents
- Approve/reject/flag users
- View risk scores and AML alerts
- See full audit trails

## Main Sections

### 1. KYC Case List
- Table of KYC requests (user, status, level, risk score, created/updated)
- Filter/search by status, level, risk, date
- Click to view case details

### 2. KYC Case Detail
- User info, submitted documents (ID, address, selfie)
- Document images and OCR data
- Verification status and provider results
- Audit trail (all actions/events)
- Approve/Reject/Request more info buttons

### 3. AML Alerts
- List of AML alerts (user, type, reason, status)
- Link to user/case detail
- Mark alert as reviewed/escalated/closed

### 4. User Profile
- KYC status, risk score, transaction history
- All submitted documents
- Manual notes and flags

### 5. Audit Trail
- Chronological log of all compliance actions
- Filter by user, event, actor, date

## Example UI Components
- KYCCaseTable
- KYCDetailPanel
- DocumentViewer
- AMLAlertTable
- UserProfilePanel
- AuditTrailTable

## Security & Compliance
- RBAC: Only compliance officers/admins can access
- All actions logged for audit
- Sensitive data masked unless needed

---
This structure can be implemented in React, Vue, or Angular, and connected to the Go API endpoints for KYC/AML management.
