package standards

import (
	"unheaded/pkg/compliance/controls"
)

// GDPRStandard implements the GDPR compliance standard.
type GDPRStandard struct {
	*BaseStandard
}

// NewGDPRStandard creates a new GDPR standard with all controls.
func NewGDPRStandard() *GDPRStandard {
	base := NewBaseStandard(
		"GDPR",
		"General Data Protection Regulation",
		"2016/679",
		"EU regulation on data protection and privacy for individuals",
	)

	s := &GDPRStandard{BaseStandard: base}
	s.initializeControls()
	return s
}

func (s *GDPRStandard) initializeControls() {
	s.addDataProcessingControls()
	s.addDataSubjectRightsControls()
	s.addSecurityControls()
	s.addAccountabilityControls()
	s.addTransferControls()
	s.addPIIContainmentControls()
}

func (s *GDPRStandard) addDataProcessingControls() {
	// Article 5 - Principles relating to processing
	s.AddControl(controls.NewControl(
		"Art.5(1)(a)",
		"GDPR",
		"Lawfulness, Fairness, Transparency",
		"Personal data shall be processed lawfully, fairly and in a transparent manner",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.5(1)(b)",
		"GDPR",
		"Purpose Limitation",
		"Personal data shall be collected for specified, explicit and legitimate purposes",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P4.1"))

	s.AddControl(controls.NewControl(
		"Art.5(1)(c)",
		"GDPR",
		"Data Minimization",
		"Personal data shall be adequate, relevant and limited to what is necessary",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P3.1"))

	s.AddControl(controls.NewControl(
		"Art.5(1)(d)",
		"GDPR",
		"Accuracy",
		"Personal data shall be accurate and kept up to date",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh).
		AddMapping("SOC2", "P8.1"))

	s.AddControl(controls.NewControl(
		"Art.5(1)(e)",
		"GDPR",
		"Storage Limitation",
		"Personal data shall be kept for no longer than necessary",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P5.1"))

	s.AddControl(controls.NewControl(
		"Art.5(1)(f)",
		"GDPR",
		"Integrity and Confidentiality",
		"Personal data shall be processed securely with appropriate protection",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.5(2)",
		"GDPR",
		"Accountability",
		"The controller shall be responsible for and demonstrate compliance",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	// Article 6 - Lawfulness of processing
	s.AddControl(controls.NewControl(
		"Art.6(1)",
		"GDPR",
		"Lawful Basis for Processing",
		"Processing shall be lawful only if and to the extent that a legal basis applies",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	// Article 7 - Conditions for consent
	s.AddControl(controls.NewControl(
		"Art.7(1)",
		"GDPR",
		"Demonstrable Consent",
		"The controller shall be able to demonstrate that the data subject has consented",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P2.1"))

	s.AddControl(controls.NewControl(
		"Art.7(2)",
		"GDPR",
		"Distinguishable Consent Request",
		"Consent request shall be clearly distinguishable from other matters",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"Art.7(3)",
		"GDPR",
		"Right to Withdraw Consent",
		"The data subject shall have the right to withdraw consent at any time",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	// Article 9 - Special categories of data
	s.AddControl(controls.NewControl(
		"Art.9(1)",
		"GDPR",
		"Special Categories Protection",
		"Processing of special categories of personal data is prohibited unless exception applies",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))
}

func (s *GDPRStandard) addDataSubjectRightsControls() {
	// Article 12 - Transparent information
	s.AddControl(controls.NewControl(
		"Art.12(1)",
		"GDPR",
		"Transparent Communication",
		"Information shall be provided in a concise, transparent, intelligible form",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh).
		AddMapping("SOC2", "P1.1"))

	s.AddControl(controls.NewControl(
		"Art.12(3)",
		"GDPR",
		"Response Timeframe",
		"Controller shall respond to data subject requests within one month",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 13 - Information to be provided
	s.AddControl(controls.NewControl(
		"Art.13(1)",
		"GDPR",
		"Privacy Notice at Collection",
		"Provide specified information at time of obtaining personal data",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	// Article 15 - Right of access
	s.AddControl(controls.NewControl(
		"Art.15(1)",
		"GDPR",
		"Right of Access",
		"Data subject has the right to obtain confirmation and access to personal data",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P6.1"))

	s.AddControl(controls.NewControl(
		"Art.15(3)",
		"GDPR",
		"Copy of Personal Data",
		"Controller shall provide a copy of the personal data undergoing processing",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 16 - Right to rectification
	s.AddControl(controls.NewControl(
		"Art.16",
		"GDPR",
		"Right to Rectification",
		"Data subject has the right to obtain rectification of inaccurate personal data",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 17 - Right to erasure
	s.AddControl(controls.NewControl(
		"Art.17(1)",
		"GDPR",
		"Right to Erasure",
		"Data subject has the right to obtain erasure of personal data",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.17(2)",
		"GDPR",
		"Notification of Erasure",
		"Controller shall inform other controllers about erasure requests",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 18 - Right to restriction
	s.AddControl(controls.NewControl(
		"Art.18(1)",
		"GDPR",
		"Right to Restriction",
		"Data subject has the right to obtain restriction of processing",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 20 - Right to data portability
	s.AddControl(controls.NewControl(
		"Art.20(1)",
		"GDPR",
		"Right to Data Portability",
		"Data subject has the right to receive personal data in a portable format",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 21 - Right to object
	s.AddControl(controls.NewControl(
		"Art.21(1)",
		"GDPR",
		"Right to Object",
		"Data subject has the right to object to processing of personal data",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	// Article 22 - Automated decision-making
	s.AddControl(controls.NewControl(
		"Art.22(1)",
		"GDPR",
		"Automated Decision-Making Protection",
		"Data subject has right not to be subject to solely automated decisions",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))
}

func (s *GDPRStandard) addSecurityControls() {
	// Article 32 - Security of processing
	s.AddControl(controls.NewControl(
		"Art.32(1)",
		"GDPR",
		"Appropriate Security Measures",
		"Implement appropriate technical and organisational measures to ensure security",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.32(1)(a)",
		"GDPR",
		"Pseudonymization and Encryption",
		"Implement pseudonymization and encryption of personal data",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SC-13"))

	s.AddControl(controls.NewControl(
		"Art.32(1)(b)",
		"GDPR",
		"Ongoing Confidentiality",
		"Ensure ongoing confidentiality, integrity, availability of processing systems",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.32(1)(c)",
		"GDPR",
		"Restore Availability",
		"Ability to restore availability and access to data in timely manner",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "CP-10"))

	s.AddControl(controls.NewControl(
		"Art.32(1)(d)",
		"GDPR",
		"Testing and Evaluation",
		"Process for regularly testing, assessing and evaluating effectiveness of measures",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh).
		AddMapping("NIST", "CA-7"))

	s.AddControl(controls.NewControl(
		"Art.32(2)",
		"GDPR",
		"Risk-Based Security",
		"Consider risks of processing when determining appropriate level of security",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"Art.32(4)",
		"GDPR",
		"Authorized Access Only",
		"Ensure persons acting under authority only process on instructions",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	// Article 33 - Notification of breach
	s.AddControl(controls.NewControl(
		"Art.33(1)",
		"GDPR",
		"Breach Notification to Authority",
		"Notify supervisory authority of personal data breach within 72 hours",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.33(3)",
		"GDPR",
		"Breach Notification Content",
		"Notification shall contain specified information about the breach",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"Art.33(5)",
		"GDPR",
		"Breach Documentation",
		"Document personal data breaches including facts, effects and remedial action",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	// Article 34 - Communication of breach to data subject
	s.AddControl(controls.NewControl(
		"Art.34(1)",
		"GDPR",
		"Breach Notification to Data Subject",
		"Communicate breach to data subject when likely to result in high risk",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))
}

func (s *GDPRStandard) addAccountabilityControls() {
	// Article 25 - Data protection by design and by default
	s.AddControl(controls.NewControl(
		"Art.25(1)",
		"GDPR",
		"Data Protection by Design",
		"Implement appropriate measures to implement data protection principles",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.25(2)",
		"GDPR",
		"Data Protection by Default",
		"Ensure that only necessary personal data are processed by default",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	// Article 28 - Processor
	s.AddControl(controls.NewControl(
		"Art.28(1)",
		"GDPR",
		"Processor Selection",
		"Use only processors providing sufficient guarantees for appropriate measures",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "CC9.2"))

	s.AddControl(controls.NewControl(
		"Art.28(3)",
		"GDPR",
		"Processing Contract",
		"Processing by processor shall be governed by a contract with specified terms",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityCritical))

	// Article 30 - Records of processing activities
	s.AddControl(controls.NewControl(
		"Art.30(1)",
		"GDPR",
		"Records of Processing Activities",
		"Maintain records of processing activities under controller responsibility",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	// Article 35 - Data protection impact assessment
	s.AddControl(controls.NewControl(
		"Art.35(1)",
		"GDPR",
		"Data Protection Impact Assessment",
		"Carry out DPIA when processing is likely to result in high risk",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.35(7)",
		"GDPR",
		"DPIA Content",
		"DPIA shall contain at minimum specified information",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	// Article 37 - Data Protection Officer
	s.AddControl(controls.NewControl(
		"Art.37(1)",
		"GDPR",
		"Designation of DPO",
		"Designate a data protection officer in specified circumstances",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	// Article 38 - Position of the DPO
	s.AddControl(controls.NewControl(
		"Art.38(1)",
		"GDPR",
		"DPO Involvement",
		"Involve the DPO properly and in a timely manner in all data protection matters",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	// Article 39 - Tasks of the DPO
	s.AddControl(controls.NewControl(
		"Art.39(1)",
		"GDPR",
		"DPO Tasks",
		"DPO shall have at least specified tasks",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))
}

func (s *GDPRStandard) addTransferControls() {
	// Article 44 - General principle for transfers
	s.AddControl(controls.NewControl(
		"Art.44",
		"GDPR",
		"Transfer Safeguards",
		"Transfers to third countries shall take place only under GDPR conditions",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P7.1"))

	// Article 45 - Adequacy decision
	s.AddControl(controls.NewControl(
		"Art.45(1)",
		"GDPR",
		"Adequacy Decision",
		"Transfer may take place to countries with adequate protection level",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityHigh))

	// Article 46 - Appropriate safeguards
	s.AddControl(controls.NewControl(
		"Art.46(1)",
		"GDPR",
		"Appropriate Safeguards for Transfer",
		"Transfers may take place with appropriate safeguards and enforceable rights",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"Art.46(2)",
		"GDPR",
		"Standard Contractual Clauses",
		"Appropriate safeguards may be provided by standard contractual clauses",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityHigh))
}

func (s *GDPRStandard) addPIIContainmentControls() {
	// Unheaded PII Containment — platform-enforced controls
	// "PII is radioactive. You need containment, handling and disposal procedures,
	// you need to allow users to inspect it at any time and if you accidentally
	// expose anyone to it that's a major emergency incident."
	// — hnbad (https://news.ycombinator.com/user?id=hnbad)

	s.AddControl(controls.NewControl(
		"UH-PII-1",
		"GDPR",
		"PII Containment Mode",
		"Platform-level PII containment with configurable enforcement (pii_mode: eu|strict). "+
			"When enabled, all services treat PII as radioactive material requiring containment, "+
			"handling procedures, and disposal protocols",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "PM-11").
		AddMapping("SOC2", "P3.1"))

	s.AddControl(controls.NewControl(
		"UH-PII-2",
		"GDPR",
		"PII Access Inspection",
		"Users can inspect all PII held about them at any time via self-service API. "+
			"Implements Art.15 right-of-access as an automated platform capability",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P6.1"))

	s.AddControl(controls.NewControl(
		"UH-PII-3",
		"GDPR",
		"PII Erasure on Demand",
		"Right-of-erasure (Art.17) implemented as automated platform capability. "+
			"Erasure propagates across all services and storage backends",
	).WithCategory(controls.CategoryPrivacy).WithSeverity(controls.SeverityCritical).
		AddMapping("SOC2", "P5.2"))

	s.AddControl(controls.NewControl(
		"UH-PII-4",
		"GDPR",
		"PII Exposure Breach Alert",
		"Accidental PII exposure triggers immediate emergency incident via Wotan "+
			"alerts.critical topic. Satisfies Art.33 72-hour notification window "+
			"by detecting breaches in real time",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))
}
