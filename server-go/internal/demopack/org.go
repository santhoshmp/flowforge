// Mock organization data for the Meridian Components demo pack: master-data
// entities with golden records and two deliberate governance stories (a
// suspected duplicate vendor and a tax-id mismatch) for the stewardship demo.
package demopack

import "github.com/flowforge/flowforge/internal/models"

// Org is the demo organization profile (surfaces in the runbook and summary).
const Org = "Meridian Components"

// MDM returns the organization's master-data entities. Record order matters:
// pending-stewardship records lead their entity so the UI shows the story.
func MDM() []models.MDMEntity {
	return []models.MDMEntity{
		{
			Key: "vendors", Label: "Suppliers", Icon: "Building2",
			Fields: []string{"id", "name", "country", "tax_id", "status"},
			Records: []map[string]string{
				// Suspected duplicate of V-1001 — flagged by the last invoice run.
				{"id": "V-1012", "name": "Brightway Electronics Shenzhen", "country": "CN", "tax_id": "CN-9144-0031-XQ", "status": "pending stewardship"},
				// Tax-id mismatch vs the portal — needs steward review.
				{"id": "V-1006", "name": "Veloce Logistics GmbH", "country": "DE", "tax_id": "DE-811-909-?", "status": "pending stewardship"},
				{"id": "V-1001", "name": "Shenzhen Brightway Electronics", "country": "CN", "tax_id": "CN-9144-0031-QX", "status": "golden"},
				{"id": "V-1002", "name": "TechSupply Global Pte Ltd", "country": "SG", "tax_id": "SG-2018-114K", "status": "golden"},
				{"id": "V-1003", "name": "Nordic Copper Works AB", "country": "SE", "tax_id": "SE-5567-8821", "status": "golden"},
				{"id": "V-1004", "name": "Penang Polymers Sdn Bhd", "country": "MY", "tax_id": "MY-2019-4471", "status": "golden"},
			},
		},
		{
			Key: "customers", Label: "Customers", Icon: "Briefcase",
			Fields: []string{"id", "name", "industry", "status"},
			Records: []map[string]string{
				{"id": "C-2001", "name": "Vestergaard Medical A/S", "industry": "Medical devices", "status": "golden"},
				{"id": "C-2002", "name": "Nordwind Automotiv GmbH", "industry": "Automotive", "status": "golden"},
				{"id": "C-2003", "name": "Halden Marine Systems", "industry": "Marine", "status": "golden"},
			},
		},
		{
			Key: "products", Label: "Products", Icon: "Package",
			Fields: []string{"id", "name", "sku", "status"},
			Records: []map[string]string{
				{"id": "P-3001", "name": "MCU board 40mm", "sku": "MCB-40-REV3", "status": "golden"},
				{"id": "P-3002", "name": "Copper heat spreader", "sku": "CHS-220-CU", "status": "golden"},
				{"id": "P-3003", "name": "Polymer housing IP67", "sku": "PHS-IP67-A2", "status": "golden"},
				{"id": "P-3004", "name": "Cable harness 1.2m", "sku": "CBH-12-ST", "status": "golden"},
			},
		},
		{
			Key: "employees", Label: "Employees", Icon: "Users",
			Fields: []string{"id", "name", "department", "status"},
			Records: []map[string]string{
				// The hire the onboarding demo runs for.
				{"id": "E-4417", "name": "Jonas de Vries", "department": "Operations", "status": "pending stewardship"},
				{"id": "E-4001", "name": "Aisha Khan", "department": "Finance", "status": "golden"},
				{"id": "E-4002", "name": "Marcus Webb", "department": "Procurement", "status": "golden"},
				{"id": "E-4003", "name": "Sofia Lindqvist", "department": "HR", "status": "golden"},
				{"id": "E-4004", "name": "Daniel Osei", "department": "Support", "status": "golden"},
				{"id": "E-4005", "name": "Tomas Herrera", "department": "Data Governance", "status": "golden"},
				{"id": "E-4006", "name": "Priya Raman", "department": "Finance", "status": "golden"},
				{"id": "E-4007", "name": "Elena Fischer", "department": "Operations", "status": "golden"},
			},
		},
	}
}
