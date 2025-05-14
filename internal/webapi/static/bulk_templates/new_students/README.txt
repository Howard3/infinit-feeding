# Student Bulk Upload - Template Instructions

## ZIP File Structure
Your upload must be a ZIP file with the following structure:
```
students.zip
├── students.csv  (Must use exactly this name)
└── photos/       (Directory containing student photos)
    ├── 12345678.jpg  (LRN as filename)
    ├── 23456789.jpg
    └── ...
```

## CSV File Format
The CSV file must contain the following columns:
- First Name
- Last Name
- LRN (Learner Reference Number)
- Grade Level
- Date of Birth (format: YYYYMMDD)
- Gender (M or F)
- Status (Inactive or Active)
- Sponsorship (Not Eligible or Eligible)

## Photo Requirements
- Each student MUST have a corresponding photo in the photos directory
- Photos must be named using the student's LRN (e.g., 12345678.jpg)
- Supported formats: JPG, JPEG, PNG
- Recommended size: At least 300x300 pixels, not exceeding 2MB
- Clear, front-facing headshot with good lighting

## Important Notes
1. **Status Field**: Enter "Inactive" if you want the student record to be created but not active yet. Enter "Active" if the student should be immediately active in the system.

2. **Sponsorship Field**: Enter "Not Eligible" if the student is not eligible for sponsorship. Enter "Eligible" if the student is eligible.

3. **LRN**: This is used to link the student record with their photo. Make sure it's unique and matches exactly between the CSV and the photo filename.

4. **Date Format**: Dates must be in YYYYMMDD format (e.g., 20120315 for March 15, 2012).

5. Make sure there are no extra spaces, commas, or special characters in your CSV entries.

For any questions or support, please contact the system administrator.