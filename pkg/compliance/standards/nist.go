package standards

import (
	"github.com/unheaded/unheaded/pkg/compliance/controls"
)

// NISTStandard implements the NIST 800-53 compliance standard.
type NISTStandard struct {
	*BaseStandard
}

// NewNISTStandard creates a new NIST 800-53 standard with all controls.
func NewNISTStandard() *NISTStandard {
	base := NewBaseStandard(
		"NIST-800-53",
		"NIST Special Publication 800-53",
		"Rev 5",
		"Security and Privacy Controls for Information Systems and Organizations",
	)

	s := &NISTStandard{BaseStandard: base}
	s.initializeControls()
	return s
}

func (s *NISTStandard) initializeControls() {
	s.addAccessControls()
	s.addAuditControls()
	s.addSecurityAssessmentControls()
	s.addConfigurationControls()
	s.addContingencyControls()
	s.addIdentificationControls()
	s.addIncidentResponseControls()
	s.addSystemControls()
}

func (s *NISTStandard) addAccessControls() {
	// AC - Access Control Family
	s.AddControl(controls.NewControl(
		"AC-1",
		"NIST-800-53",
		"Access Control Policy and Procedures",
		"The organization develops, documents, and disseminates an access control policy",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh).
		WithRequirements(
			"Document access control policy",
			"Define access control procedures",
			"Review and update policy annually",
		))

	s.AddControl(controls.NewControl(
		"AC-2",
		"NIST-800-53",
		"Account Management",
		"The organization manages information system accounts",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC6.1").
		AddMapping("PCI-DSS", "8.1"))

	s.AddControl(controls.NewControl(
		"AC-3",
		"NIST-800-53",
		"Access Enforcement",
		"The system enforces approved authorizations for logical access",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"AC-4",
		"NIST-800-53",
		"Information Flow Enforcement",
		"The system enforces approved authorizations for controlling information flow",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AC-5",
		"NIST-800-53",
		"Separation of Duties",
		"The organization separates duties of individuals to prevent malicious activity",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AC-6",
		"NIST-800-53",
		"Least Privilege",
		"The organization employs the principle of least privilege",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("PCI-DSS", "7.1"))

	s.AddControl(controls.NewControl(
		"AC-7",
		"NIST-800-53",
		"Unsuccessful Logon Attempts",
		"The system enforces a limit on consecutive invalid logon attempts",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AC-8",
		"NIST-800-53",
		"System Use Notification",
		"The system displays a notification banner before granting access",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"AC-11",
		"NIST-800-53",
		"Session Lock",
		"The system initiates a session lock after a period of inactivity",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"AC-17",
		"NIST-800-53",
		"Remote Access",
		"The organization establishes usage restrictions for remote access",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AC-18",
		"NIST-800-53",
		"Wireless Access",
		"The organization establishes usage restrictions for wireless access",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AC-20",
		"NIST-800-53",
		"Use of External Systems",
		"The organization establishes terms for authorized use of external systems",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityMedium))
}

func (s *NISTStandard) addAuditControls() {
	// AU - Audit and Accountability Family
	s.AddControl(controls.NewControl(
		"AU-1",
		"NIST-800-53",
		"Audit Policy and Procedures",
		"The organization develops audit and accountability policy and procedures",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AU-2",
		"NIST-800-53",
		"Audit Events",
		"The organization determines auditable events for the system",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC7.2"))

	s.AddControl(controls.NewControl(
		"AU-3",
		"NIST-800-53",
		"Content of Audit Records",
		"The system generates audit records containing required information",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AU-4",
		"NIST-800-53",
		"Audit Storage Capacity",
		"The organization allocates audit record storage capacity",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"AU-5",
		"NIST-800-53",
		"Response to Audit Processing Failures",
		"The system alerts personnel in event of audit processing failure",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"AU-6",
		"NIST-800-53",
		"Audit Review, Analysis, and Reporting",
		"The organization reviews and analyzes audit records for anomalies",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"AU-9",
		"NIST-800-53",
		"Protection of Audit Information",
		"The system protects audit information and tools from unauthorized access",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"AU-11",
		"NIST-800-53",
		"Audit Record Retention",
		"The organization retains audit records for the required retention period",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh).
		AddMapping("PCI-DSS", "10.7"))

	s.AddControl(controls.NewControl(
		"AU-12",
		"NIST-800-53",
		"Audit Generation",
		"The system provides audit record generation capability",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))
}

func (s *NISTStandard) addSecurityAssessmentControls() {
	// CA - Security Assessment and Authorization
	s.AddControl(controls.NewControl(
		"CA-1",
		"NIST-800-53",
		"Security Assessment Policy",
		"The organization develops security assessment and authorization policy",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CA-2",
		"NIST-800-53",
		"Security Assessments",
		"The organization develops and implements security assessment plans",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"CA-5",
		"NIST-800-53",
		"Plan of Action and Milestones",
		"The organization develops a plan of action to correct deficiencies",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CA-7",
		"NIST-800-53",
		"Continuous Monitoring",
		"The organization develops a continuous monitoring strategy",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))
}

func (s *NISTStandard) addConfigurationControls() {
	// CM - Configuration Management
	s.AddControl(controls.NewControl(
		"CM-1",
		"NIST-800-53",
		"Configuration Management Policy",
		"The organization develops configuration management policy and procedures",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CM-2",
		"NIST-800-53",
		"Baseline Configuration",
		"The organization develops and maintains baseline configurations",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"CM-3",
		"NIST-800-53",
		"Configuration Change Control",
		"The organization analyzes changes to the system for security impact",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC8.1"))

	s.AddControl(controls.NewControl(
		"CM-6",
		"NIST-800-53",
		"Configuration Settings",
		"The organization establishes mandatory configuration settings",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CM-7",
		"NIST-800-53",
		"Least Functionality",
		"The organization configures the system to provide only essential capabilities",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CM-8",
		"NIST-800-53",
		"System Component Inventory",
		"The organization develops and maintains an inventory of system components",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityHigh))
}

func (s *NISTStandard) addContingencyControls() {
	// CP - Contingency Planning
	s.AddControl(controls.NewControl(
		"CP-1",
		"NIST-800-53",
		"Contingency Planning Policy",
		"The organization develops contingency planning policy and procedures",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CP-2",
		"NIST-800-53",
		"Contingency Plan",
		"The organization develops a contingency plan for the system",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"CP-4",
		"NIST-800-53",
		"Contingency Plan Testing",
		"The organization tests the contingency plan to determine effectiveness",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh).
		AddMapping("SOC2", "A1.3"))

	s.AddControl(controls.NewControl(
		"CP-9",
		"NIST-800-53",
		"System Backup",
		"The organization conducts backups of system data and information",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"CP-10",
		"NIST-800-53",
		"System Recovery and Reconstitution",
		"The organization provides for recovery and reconstitution of the system",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))
}

func (s *NISTStandard) addIdentificationControls() {
	// IA - Identification and Authentication
	s.AddControl(controls.NewControl(
		"IA-1",
		"NIST-800-53",
		"Identification and Authentication Policy",
		"The organization develops identification and authentication policy",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"IA-2",
		"NIST-800-53",
		"Identification and Authentication (Users)",
		"The system uniquely identifies and authenticates organizational users",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("PCI-DSS", "8.2"))

	s.AddControl(controls.NewControl(
		"IA-5",
		"NIST-800-53",
		"Authenticator Management",
		"The organization manages system authenticators",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"IA-8",
		"NIST-800-53",
		"Identification and Authentication (Non-Users)",
		"The system uniquely identifies and authenticates non-organizational users",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))
}

func (s *NISTStandard) addIncidentResponseControls() {
	// IR - Incident Response
	s.AddControl(controls.NewControl(
		"IR-1",
		"NIST-800-53",
		"Incident Response Policy",
		"The organization develops incident response policy and procedures",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"IR-4",
		"NIST-800-53",
		"Incident Handling",
		"The organization implements incident handling capability",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC7.4"))

	s.AddControl(controls.NewControl(
		"IR-5",
		"NIST-800-53",
		"Incident Monitoring",
		"The organization tracks and documents security incidents",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"IR-6",
		"NIST-800-53",
		"Incident Reporting",
		"The organization requires personnel to report suspected incidents",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"IR-8",
		"NIST-800-53",
		"Incident Response Plan",
		"The organization develops an incident response plan",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))
}

func (s *NISTStandard) addSystemControls() {
	// SC - System and Communications Protection
	s.AddControl(controls.NewControl(
		"SC-7",
		"NIST-800-53",
		"Boundary Protection",
		"The system monitors and controls communications at external boundaries",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC6.6"))

	s.AddControl(controls.NewControl(
		"SC-8",
		"NIST-800-53",
		"Transmission Confidentiality and Integrity",
		"The system protects the confidentiality and integrity of transmitted information",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC6.7").
		AddMapping("PCI-DSS", "4.1"))

	s.AddControl(controls.NewControl(
		"SC-12",
		"NIST-800-53",
		"Cryptographic Key Management",
		"The organization establishes and manages cryptographic keys",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"SC-13",
		"NIST-800-53",
		"Cryptographic Protection",
		"The system implements cryptographic mechanisms",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("PCI-DSS", "3.4"))

	// SI - System and Information Integrity
	s.AddControl(controls.NewControl(
		"SI-2",
		"NIST-800-53",
		"Flaw Remediation",
		"The organization identifies, reports, and corrects system flaws",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical).
		AddMapping("PCI-DSS", "6.2"))

	s.AddControl(controls.NewControl(
		"SI-3",
		"NIST-800-53",
		"Malicious Code Protection",
		"The organization employs malicious code protection mechanisms",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical).
		AddMapping("PCI-DSS", "5.1"))

	s.AddControl(controls.NewControl(
		"SI-4",
		"NIST-800-53",
		"System Monitoring",
		"The organization monitors the system to detect attacks and anomalies",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC7.1"))
}
