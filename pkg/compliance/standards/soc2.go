package standards

import (
	"github.com/unheaded/unheaded/pkg/compliance/controls"
)

// SOC2Standard implements the SOC 2 Type II compliance standard.
type SOC2Standard struct {
	*BaseStandard
}

// NewSOC2Standard creates a new SOC 2 standard with all controls.
func NewSOC2Standard() *SOC2Standard {
	base := NewBaseStandard(
		"SOC2",
		"SOC 2 Type II",
		"2017",
		"Service Organization Control 2 - Trust Services Criteria",
	)

	s := &SOC2Standard{BaseStandard: base}
	s.initializeControls()
	return s
}

func (s *SOC2Standard) initializeControls() {
	// Security - Common Criteria (CC)
	s.addSecurityControls()

	// Availability
	s.addAvailabilityControls()

	// Processing Integrity
	s.addProcessingIntegrityControls()

	// Confidentiality
	s.addConfidentialityControls()

	// Privacy
	s.addPrivacyControls()
}

func (s *SOC2Standard) addSecurityControls() {
	// CC1 - Control Environment
	s.AddControl(controls.NewControl(
		"CC1.1",
		"SOC2",
		"Commitment to Integrity and Ethics",
		"The entity demonstrates a commitment to integrity and ethical values",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC1.2",
		"SOC2",
		"Board Independence and Oversight",
		"The board of directors demonstrates independence from management and exercises oversight",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityMedium))

	// CC2 - Communication and Information
	s.AddControl(controls.NewControl(
		"CC2.1",
		"SOC2",
		"Internal Communication",
		"The entity internally communicates information necessary to support internal control",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"CC2.2",
		"SOC2",
		"External Communication",
		"The entity externally communicates information necessary to meet commitments",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityMedium))

	// CC3 - Risk Assessment
	s.AddControl(controls.NewControl(
		"CC3.1",
		"SOC2",
		"Risk Objectives",
		"The entity specifies objectives with sufficient clarity to identify risks",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC3.2",
		"SOC2",
		"Risk Identification",
		"The entity identifies risks to the achievement of objectives",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC3.3",
		"SOC2",
		"Fraud Risk Assessment",
		"The entity considers the potential for fraud in assessing risks",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	// CC5 - Control Activities
	s.AddControl(controls.NewControl(
		"CC5.1",
		"SOC2",
		"Control Activity Selection",
		"The entity selects and develops control activities that mitigate risks",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC5.2",
		"SOC2",
		"Technology Controls",
		"The entity selects and develops general control activities over technology",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityHigh))

	// CC6 - Logical and Physical Access
	s.AddControl(controls.NewControl(
		"CC6.1",
		"SOC2",
		"Access Security",
		"The entity implements logical access security software and infrastructure",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AC-2"))

	s.AddControl(controls.NewControl(
		"CC6.2",
		"SOC2",
		"Access Registration",
		"Prior to issuing system credentials, the entity registers and authorizes new users",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"CC6.3",
		"SOC2",
		"Access Removal",
		"The entity removes access to protected information when no longer required",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC6.6",
		"SOC2",
		"System Boundary Protection",
		"The entity implements logical access security measures to protect system boundaries",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"CC6.7",
		"SOC2",
		"Data Transmission Protection",
		"The entity restricts the transmission of data to authorized channels",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	// CC7 - System Operations
	s.AddControl(controls.NewControl(
		"CC7.1",
		"SOC2",
		"Vulnerability Detection",
		"The entity uses detection and monitoring procedures to identify vulnerabilities",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC7.2",
		"SOC2",
		"Anomaly Detection",
		"The entity monitors system components and operations to detect anomalies",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC7.3",
		"SOC2",
		"Security Incident Evaluation",
		"The entity evaluates security events to determine whether they are security incidents",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC7.4",
		"SOC2",
		"Incident Response",
		"The entity responds to identified security incidents by following procedures",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))

	// CC8 - Change Management
	s.AddControl(controls.NewControl(
		"CC8.1",
		"SOC2",
		"Change Management Process",
		"The entity authorizes, designs, develops, configures, and implements changes",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityHigh))

	// CC9 - Risk Mitigation
	s.AddControl(controls.NewControl(
		"CC9.1",
		"SOC2",
		"Risk Mitigation",
		"The entity identifies, selects, and develops risk mitigation activities",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"CC9.2",
		"SOC2",
		"Vendor Risk Management",
		"The entity assesses and manages risks associated with vendors and partners",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityHigh))
}

func (s *SOC2Standard) addAvailabilityControls() {
	s.AddControl(controls.NewControl(
		"A1.1",
		"SOC2",
		"Capacity Planning",
		"The entity maintains, monitors, and evaluates current processing capacity",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"A1.2",
		"SOC2",
		"Recovery Procedures",
		"The entity authorizes, designs, and implements recovery procedures",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"A1.3",
		"SOC2",
		"Recovery Testing",
		"The entity tests recovery plan procedures supporting system recovery",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))
}

func (s *SOC2Standard) addProcessingIntegrityControls() {
	s.AddControl(controls.NewControl(
		"PI1.1",
		"SOC2",
		"Processing Accuracy",
		"The entity obtains or generates, uses, and communicates accurate information",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"PI1.2",
		"SOC2",
		"Processing Completeness",
		"The entity implements policies and procedures over system processing completeness",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"PI1.3",
		"SOC2",
		"Processing Timeliness",
		"The entity implements policies and procedures over system processing timeliness",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityMedium))
}

func (s *SOC2Standard) addConfidentialityControls() {
	s.AddControl(controls.NewControl(
		"C1.1",
		"SOC2",
		"Confidential Information Identification",
		"The entity identifies and maintains confidential information",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"C1.2",
		"SOC2",
		"Confidential Information Disposal",
		"The entity disposes of confidential information following commitments",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))
}

func (s *SOC2Standard) addPrivacyControls() {
	s.AddControl(controls.NewControl(
		"P1.1",
		"SOC2",
		"Privacy Notice",
		"The entity provides notice about its privacy practices",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"P2.1",
		"SOC2",
		"Consent",
		"The entity obtains consent for the collection, use, and disclosure of personal information",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"P3.1",
		"SOC2",
		"Collection Limitation",
		"The entity collects personal information only for the purposes identified in the notice",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"P4.1",
		"SOC2",
		"Use Limitation",
		"The entity limits the use of personal information to identified purposes",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"P5.1",
		"SOC2",
		"Data Retention",
		"The entity retains personal information for only as long as necessary",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"P6.1",
		"SOC2",
		"Access Rights",
		"The entity provides individuals with access to their personal information",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"P7.1",
		"SOC2",
		"Disclosure to Third Parties",
		"The entity discloses personal information to third parties only with consent",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"P8.1",
		"SOC2",
		"Data Quality",
		"The entity maintains accurate and complete personal information",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityMedium))
}
