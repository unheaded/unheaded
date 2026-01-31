package standards

import (
	"github.com/unheaded/unheaded/pkg/compliance/controls"
)

// PCIStandard implements the PCI-DSS compliance standard.
type PCIStandard struct {
	*BaseStandard
}

// NewPCIStandard creates a new PCI-DSS standard with all controls.
func NewPCIStandard() *PCIStandard {
	base := NewBaseStandard(
		"PCI-DSS",
		"Payment Card Industry Data Security Standard",
		"4.0",
		"Security standard for organizations handling credit card data",
	)

	s := &PCIStandard{BaseStandard: base}
	s.initializeControls()
	return s
}

func (s *PCIStandard) initializeControls() {
	s.addNetworkSecurityControls()
	s.addDataProtectionControls()
	s.addVulnerabilityManagementControls()
	s.addAccessControlControls()
	s.addMonitoringControls()
	s.addSecurityPolicyControls()
}

func (s *PCIStandard) addNetworkSecurityControls() {
	// Requirement 1: Install and maintain network security controls
	s.AddControl(controls.NewControl(
		"1.1",
		"PCI-DSS",
		"Network Security Controls",
		"Processes and mechanisms for installing and maintaining network security controls are defined and understood",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"1.2",
		"PCI-DSS",
		"Network Security Control Configuration",
		"Network security controls are configured and maintained",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SC-7"))

	s.AddControl(controls.NewControl(
		"1.3",
		"PCI-DSS",
		"Network Access Restriction",
		"Network access to and from the cardholder data environment is restricted",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"1.4",
		"PCI-DSS",
		"Network Connections Control",
		"Network connections between trusted and untrusted networks are controlled",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"1.5",
		"PCI-DSS",
		"Portable Computing Devices",
		"Risks to the CDE from computing devices connecting to networks are mitigated",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityHigh))
}

func (s *PCIStandard) addDataProtectionControls() {
	// Requirement 3: Protect stored account data
	s.AddControl(controls.NewControl(
		"3.1",
		"PCI-DSS",
		"Stored Account Data Protection",
		"Processes and mechanisms for protecting stored account data are defined and understood",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"3.2",
		"PCI-DSS",
		"Data Retention Minimization",
		"Storage of account data is kept to a minimum",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"3.3",
		"PCI-DSS",
		"Sensitive Authentication Data",
		"Sensitive authentication data (SAD) is not stored after authorization",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"3.4",
		"PCI-DSS",
		"PAN Rendering Unreadable",
		"Access to displays of full PAN and ability to copy PAN is restricted",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SC-13"))

	s.AddControl(controls.NewControl(
		"3.5",
		"PCI-DSS",
		"PAN Encryption",
		"Primary account number (PAN) is secured wherever it is stored",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"3.6",
		"PCI-DSS",
		"Cryptographic Key Management",
		"Cryptographic keys used to protect stored account data are secured",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"3.7",
		"PCI-DSS",
		"Key Management Procedures",
		"Cryptographic key management procedures are fully documented and implemented",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityHigh))

	// Requirement 4: Protect cardholder data with strong cryptography during transmission
	s.AddControl(controls.NewControl(
		"4.1",
		"PCI-DSS",
		"Transmission Protection",
		"Processes and mechanisms for protecting cardholder data during transmission are defined",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SC-8"))

	s.AddControl(controls.NewControl(
		"4.2",
		"PCI-DSS",
		"Strong Cryptography for Transmission",
		"PAN is protected with strong cryptography during transmission",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))
}

func (s *PCIStandard) addVulnerabilityManagementControls() {
	// Requirement 5: Protect all systems and networks from malicious software
	s.AddControl(controls.NewControl(
		"5.1",
		"PCI-DSS",
		"Anti-Malware Solutions",
		"Processes and mechanisms for protecting systems from malicious software are defined",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SI-3"))

	s.AddControl(controls.NewControl(
		"5.2",
		"PCI-DSS",
		"Anti-Malware Deployment",
		"Malicious software is prevented or detected and addressed",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"5.3",
		"PCI-DSS",
		"Anti-Malware Mechanisms",
		"Anti-malware mechanisms and processes are active, maintained, and monitored",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"5.4",
		"PCI-DSS",
		"Phishing Protection",
		"Anti-phishing mechanisms protect users against phishing attacks",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityHigh))

	// Requirement 6: Develop and maintain secure systems and software
	s.AddControl(controls.NewControl(
		"6.1",
		"PCI-DSS",
		"Secure Development Processes",
		"Processes and mechanisms for secure development are defined and understood",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"6.2",
		"PCI-DSS",
		"Security Patches",
		"Bespoke and custom software is developed securely",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "SI-2"))

	s.AddControl(controls.NewControl(
		"6.3",
		"PCI-DSS",
		"Security Vulnerabilities",
		"Security vulnerabilities are identified and addressed",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"6.4",
		"PCI-DSS",
		"Public-Facing Web Applications",
		"Public-facing web applications are protected against attacks",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"6.5",
		"PCI-DSS",
		"Change Management",
		"Changes to all system components are managed securely",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityHigh))
}

func (s *PCIStandard) addAccessControlControls() {
	// Requirement 7: Restrict access to system components and cardholder data
	s.AddControl(controls.NewControl(
		"7.1",
		"PCI-DSS",
		"Access Control Processes",
		"Processes for restricting access to system components and cardholder data are defined",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AC-6"))

	s.AddControl(controls.NewControl(
		"7.2",
		"PCI-DSS",
		"Access Control Model",
		"Access to system components and data is appropriately defined and assigned",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"7.3",
		"PCI-DSS",
		"Access Control Enforcement",
		"Access to system components and data is managed via an access control system",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	// Requirement 8: Identify users and authenticate access
	s.AddControl(controls.NewControl(
		"8.1",
		"PCI-DSS",
		"User Identification",
		"Processes for identifying users and authenticating access are defined and understood",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AC-2"))

	s.AddControl(controls.NewControl(
		"8.2",
		"PCI-DSS",
		"User Authentication",
		"User identification and related accounts are strictly managed",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "IA-2"))

	s.AddControl(controls.NewControl(
		"8.3",
		"PCI-DSS",
		"Strong Authentication",
		"Strong authentication for users and administrators is established and managed",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"8.4",
		"PCI-DSS",
		"Multi-Factor Authentication",
		"Multi-factor authentication is implemented to secure access",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"8.5",
		"PCI-DSS",
		"Multi-User Accounts",
		"Multi-factor authentication (MFA) systems are configured securely",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"8.6",
		"PCI-DSS",
		"Application and System Accounts",
		"Use of application and system accounts is strictly managed",
	).WithCategory(controls.CategoryAccessControl).WithSeverity(controls.SeverityHigh))

	// Requirement 9: Restrict physical access to cardholder data
	s.AddControl(controls.NewControl(
		"9.1",
		"PCI-DSS",
		"Physical Access Processes",
		"Processes for restricting physical access to cardholder data are defined",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"9.2",
		"PCI-DSS",
		"Physical Entry Controls",
		"Physical access controls manage entry into facilities and systems",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"9.3",
		"PCI-DSS",
		"Personnel Physical Access",
		"Physical access for personnel and visitors is authorized and managed",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"9.4",
		"PCI-DSS",
		"Media Protection",
		"Media with cardholder data is securely stored, accessed, distributed, and destroyed",
	).WithCategory(controls.CategoryDataProtection).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"9.5",
		"PCI-DSS",
		"POI Device Protection",
		"Point of interaction (POI) devices are protected from tampering and substitution",
	).WithCategory(controls.CategoryPhysicalSecurity).WithSeverity(controls.SeverityCritical))
}

func (s *PCIStandard) addMonitoringControls() {
	// Requirement 10: Log and monitor all access to system components and cardholder data
	s.AddControl(controls.NewControl(
		"10.1",
		"PCI-DSS",
		"Logging Processes",
		"Processes for logging and monitoring are defined and understood",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"10.2",
		"PCI-DSS",
		"Audit Logs",
		"Audit logs are implemented to support detection of anomalies",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"10.3",
		"PCI-DSS",
		"Audit Log Protection",
		"Audit logs are protected from destruction and unauthorized modifications",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"10.4",
		"PCI-DSS",
		"Audit Log Review",
		"Audit logs are reviewed to identify anomalies or suspicious activity",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"10.5",
		"PCI-DSS",
		"Audit Log History",
		"Audit log history is retained and available for analysis",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"10.6",
		"PCI-DSS",
		"Time Synchronization",
		"Time-synchronization mechanisms support consistent time across all systems",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"10.7",
		"PCI-DSS",
		"Critical Control Failures",
		"Failures of critical security control systems are detected and addressed",
	).WithCategory(controls.CategoryAuditLogging).WithSeverity(controls.SeverityCritical).
		AddMapping("NIST", "AU-11"))

	// Requirement 11: Test security of systems and networks regularly
	s.AddControl(controls.NewControl(
		"11.1",
		"PCI-DSS",
		"Security Testing Processes",
		"Processes for regular testing of security are defined and understood",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"11.2",
		"PCI-DSS",
		"Wireless Access Points",
		"Wireless access points are identified and monitored",
	).WithCategory(controls.CategoryNetworkSecurity).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"11.3",
		"PCI-DSS",
		"Vulnerability Scanning",
		"External and internal vulnerabilities are regularly identified and addressed",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"11.4",
		"PCI-DSS",
		"Penetration Testing",
		"External and internal penetration testing is regularly performed",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"11.5",
		"PCI-DSS",
		"Intrusion Detection",
		"Network intrusions and unexpected file changes are detected and responded to",
	).WithCategory(controls.CategorySystemIntegrity).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"11.6",
		"PCI-DSS",
		"Change Detection",
		"Unauthorized changes on payment pages are detected and responded to",
	).WithCategory(controls.CategoryChangeManagement).WithSeverity(controls.SeverityCritical))
}

func (s *PCIStandard) addSecurityPolicyControls() {
	// Requirement 12: Support information security with organizational policies and programs
	s.AddControl(controls.NewControl(
		"12.1",
		"PCI-DSS",
		"Information Security Policy",
		"A comprehensive information security policy that governs and guides protection is known",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"12.2",
		"PCI-DSS",
		"Acceptable Use Policies",
		"Acceptable use policies for end-user technologies are defined and implemented",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"12.3",
		"PCI-DSS",
		"Risk Assessment",
		"Risks to the cardholder data environment are formally identified and managed",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"12.4",
		"PCI-DSS",
		"PCI DSS Compliance Program",
		"PCI DSS compliance is managed throughout the entity",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"12.5",
		"PCI-DSS",
		"PCI DSS Scope",
		"PCI DSS scope is documented and validated",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"12.6",
		"PCI-DSS",
		"Security Awareness Training",
		"Security awareness education is an ongoing activity",
	).WithCategory(controls.CategorySecurityAwareness).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"12.7",
		"PCI-DSS",
		"Personnel Screening",
		"Personnel are screened to reduce risks from insider threats",
	).WithCategory(controls.CategoryRiskManagement).WithSeverity(controls.SeverityMedium))

	s.AddControl(controls.NewControl(
		"12.8",
		"PCI-DSS",
		"Third-Party Service Provider",
		"Risk to information assets associated with TPSP relationships is managed",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityCritical))

	s.AddControl(controls.NewControl(
		"12.9",
		"PCI-DSS",
		"TPSP Compliance Support",
		"Third-party service providers support their customers' PCI DSS compliance",
	).WithCategory(controls.CategoryVendorManagement).WithSeverity(controls.SeverityHigh))

	s.AddControl(controls.NewControl(
		"12.10",
		"PCI-DSS",
		"Incident Response",
		"Suspected and confirmed security incidents are responded to immediately",
	).WithCategory(controls.CategoryIncidentResponse).WithSeverity(controls.SeverityCritical))
}
