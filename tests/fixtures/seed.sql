-- Test fixture for Branchd E2E tests
-- This data will be restored to branches and verified for anonymization

DROP TABLE IF EXISTS users CASCADE;

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100),
    phone VARCHAR(20),
    address TEXT,
    ssn VARCHAR(11)
);

-- Insert test users with sensitive PII data
-- These will be anonymized by anon rules during restore
INSERT INTO users (name, email, phone, address, ssn) VALUES
    ('Alice Johnson', 'alice.johnson@company.com', '+1-555-0101', '123 Main St, New York, NY 10001', '123-45-6789'),
    ('Bob Smith', 'bob.smith@company.com', '+1-555-0102', '456 Oak Ave, Los Angeles, CA 90001', '234-56-7890'),
    ('Carol Williams', 'carol.williams@company.com', '+1-555-0103', '789 Pine Rd, Chicago, IL 60601', '345-67-8901');
