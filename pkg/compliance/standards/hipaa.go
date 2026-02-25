// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package standards

import (
	"unheaded/pkg/compliance/controls"
)

// HIPAAStandard implements the HIPAA compliance standard.
type HIPAAStandard struct {
	*BaseStandard
}

// NewHIPAAStandard creates a new HIPAA standard with all controls.
func NewHIPAAStandard() *HIPAAStandard {
	base := NewBaseStandard(
		"HIPAA",
		"Health Insurance Portability and Accountability Act",
		"2013",
		"Security Rule requirements for protecting electronic protected health information",
	)

	s := &HIPAAStandard{BaseStandard: base}
	s.initializeControls()
	return s
}

func (s *HIPAAStandard) initializeControls() {
	s.addAdministrativeSafeguards()
	s.addPhysicalSafeguards()
	s.addTechnicalSafeguards()
}

func (s *HIPAAStandard) addAdministrativeSafeguards() {
	// 164.308 - Administrative Safeguards
	s.AddControl(controls.NewControl(
		"164.308(a)(1)",
		"HIPAA",
		"Security Management Process",
		"Implement policies and procedures to prevent, detect, contain, and correct security violations",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(1)(i)",
		"HIPAA",
		"Risk Analysis",
		"Conduct an accurate and thorough assessment of potential risks to ePHI",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "CA-2"))

	s.AddControl(controls.NewControl(
		"164.308(a)(1)(ii)(A)",
		"HIPAA",
		"Risk Management",
		"Implement security measures to reduce risks and vulnerabilities",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(1)(ii)(B)",
		"HIPAA",
		"Sanction Policy",
		"Apply appropriate sanctions against workforce members who fail to comply",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(a)(1)(ii)(C)",
		"HIPAA",
		"Information System Activity Review",
		"Implement procedures to regularly review records of information system activity",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AU-6"))

	s.AddControl(controls.NewControl(
		"164.308(a)(2)",
		"HIPAA",
		"Assigned Security Responsibility",
		"Identify the security official responsible for developing and implementing policies",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(a)(3)",
		"HIPAA",
		"Workforce Security",
		"Implement policies and procedures to ensure workforce members have appropriate access",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(3)(ii)(A)",
		"HIPAA",
		"Authorization and Supervision",
		"Implement procedures for authorization and supervision of workforce members",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(a)(3)(ii)(B)",
		"HIPAA",
		"Workforce Clearance Procedure",
		"Implement procedures to determine appropriate level of access",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(a)(3)(ii)(C)",
		"HIPAA",
		"Termination Procedures",
		"Implement procedures for terminating access when employment ends",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC6.3"))

	s.AddControl(controls.NewControl(
		"164.308(a)(4)",
		"HIPAA",
		"Information Access Management",
		"Implement policies and procedures for authorizing access to ePHI",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(5)",
		"HIPAA",
		"Security Awareness and Training",
		"Implement a security awareness and training program for all workforce members",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(5)(ii)(A)",
		"HIPAA",
		"Security Reminders",
		"Implement periodic security updates and reminders",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"164.308(a)(5)(ii)(B)",
		"HIPAA",
		"Protection from Malicious Software",
		"Implement procedures for guarding against and detecting malicious software",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SI-3"))

	s.AddControl(controls.NewControl(
		"164.308(a)(5)(ii)(C)",
		"HIPAA",
		"Log-in Monitoring",
		"Implement procedures for monitoring log-in attempts and reporting discrepancies",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(a)(5)(ii)(D)",
		"HIPAA",
		"Password Management",
		"Implement procedures for creating, changing, and safeguarding passwords",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(a)(6)",
		"HIPAA",
		"Security Incident Procedures",
		"Implement policies and procedures to address security incidents",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "IR-4"))

	s.AddControl(controls.NewControl(
		"164.308(a)(7)",
		"HIPAA",
		"Contingency Plan",
		"Establish policies and procedures for responding to emergencies",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "CP-2"))

	s.AddControl(controls.NewControl(
		"164.308(a)(7)(ii)(A)",
		"HIPAA",
		"Data Backup Plan",
		"Establish procedures to create and maintain retrievable exact copies of ePHI",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "CP-9"))

	s.AddControl(controls.NewControl(
		"164.308(a)(7)(ii)(B)",
		"HIPAA",
		"Disaster Recovery Plan",
		"Establish procedures to restore any loss of data",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(7)(ii)(C)",
		"HIPAA",
		"Emergency Mode Operation Plan",
		"Establish procedures to enable continuation of critical business processes",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.308(a)(7)(ii)(D)",
		"HIPAA",
		"Testing and Revision Procedures",
		"Implement procedures for periodic testing and revision of contingency plans",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh).
		AddMapping("NIST", "CP-4"))

	s.AddControl(controls.NewControl(
		"164.308(a)(8)",
		"HIPAA",
		"Evaluation",
		"Perform periodic technical and non-technical evaluations",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.308(b)(1)",
		"HIPAA",
		"Business Associate Contracts",
		"Obtain satisfactory assurances from business associates",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityCritical))
}

func (s *HIPAAStandard) addPhysicalSafeguards() {
	// 164.310 - Physical Safeguards
	s.AddControl(controls.NewControl(
		"164.310(a)(1)",
		"HIPAA",
		"Facility Access Controls",
		"Implement policies to limit physical access to ePHI systems",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.310(a)(2)(i)",
		"HIPAA",
		"Contingency Operations",
		"Establish procedures for facility access in support of data restoration",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.310(a)(2)(ii)",
		"HIPAA",
		"Facility Security Plan",
		"Implement policies to safeguard the facility and equipment from unauthorized access",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.310(a)(2)(iii)",
		"HIPAA",
		"Access Control and Validation Procedures",
		"Implement procedures to control and validate physical access",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.310(a)(2)(iv)",
		"HIPAA",
		"Maintenance Records",
		"Implement policies for documenting repairs and modifications to facility",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"164.310(b)",
		"HIPAA",
		"Workstation Use",
		"Implement policies specifying proper functions and physical attributes of workstations",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.310(c)",
		"HIPAA",
		"Workstation Security",
		"Implement physical safeguards for workstations accessing ePHI",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.310(d)(1)",
		"HIPAA",
		"Device and Media Controls",
		"Implement policies for receipt and removal of hardware and electronic media",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.310(d)(2)(i)",
		"HIPAA",
		"Disposal",
		"Implement policies for final disposal of ePHI and hardware",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.310(d)(2)(ii)",
		"HIPAA",
		"Media Re-use",
		"Implement procedures for removal of ePHI before re-use of media",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.310(d)(2)(iii)",
		"HIPAA",
		"Accountability",
		"Maintain record of movements of hardware and electronic media",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.310(d)(2)(iv)",
		"HIPAA",
		"Data Backup and Storage",
		"Create retrievable exact copy of ePHI before movement of equipment",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))
}

func (s *HIPAAStandard) addTechnicalSafeguards() {
	// 164.312 - Technical Safeguards
	s.AddControl(controls.NewControl(
		"164.312(a)(1)",
		"HIPAA",
		"Access Control",
		"Implement technical policies to allow access only to authorized persons",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AC-3"))

	s.AddControl(controls.NewControl(
		"164.312(a)(2)(i)",
		"HIPAA",
		"Unique User Identification",
		"Assign unique name and number for identifying and tracking user identity",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "IA-2"))

	s.AddControl(controls.NewControl(
		"164.312(a)(2)(ii)",
		"HIPAA",
		"Emergency Access Procedure",
		"Establish procedures for obtaining necessary ePHI during an emergency",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.312(a)(2)(iii)",
		"HIPAA",
		"Automatic Logoff",
		"Implement procedures that terminate sessions after predetermined inactivity",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityMedium).
		AddMapping("NIST", "AC-11"))

	s.AddControl(controls.NewControl(
		"164.312(a)(2)(iv)",
		"HIPAA",
		"Encryption and Decryption",
		"Implement mechanism to encrypt and decrypt ePHI",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SC-13"))

	s.AddControl(controls.NewControl(
		"164.312(b)",
		"HIPAA",
		"Audit Controls",
		"Implement hardware, software, and procedures to record and examine activity",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AU-2"))

	s.AddControl(controls.NewControl(
		"164.312(c)(1)",
		"HIPAA",
		"Integrity",
		"Implement policies to protect ePHI from improper alteration or destruction",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"164.312(c)(2)",
		"HIPAA",
		"Mechanism to Authenticate ePHI",
		"Implement electronic mechanisms to corroborate that ePHI has not been altered",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.312(d)",
		"HIPAA",
		"Person or Entity Authentication",
		"Implement procedures to verify that a person seeking access is the claimed one",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "IA-5"))

	s.AddControl(controls.NewControl(
		"164.312(e)(1)",
		"HIPAA",
		"Transmission Security",
		"Implement technical security measures to guard against unauthorized access during transmission",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SC-8"))

	s.AddControl(controls.NewControl(
		"164.312(e)(2)(i)",
		"HIPAA",
		"Integrity Controls",
		"Implement security measures to ensure electronically transmitted ePHI is not improperly modified",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"164.312(e)(2)(ii)",
		"HIPAA",
		"Encryption",
		"Implement mechanism to encrypt ePHI whenever deemed appropriate",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))
}
