CREATE TABLE active_checkins (
    employee_id VARCHAR(50) PRIMARY KEY,
    factory_id VARCHAR(50) NOT NULL,
    location_tz VARCHAR(100) NOT NULL,
    check_in_time TIMESTAMPTZ NOT NULL
);

CREATE TABLE weekly_totals (
    week_key VARCHAR(100) PRIMARY KEY, -- format: "EmployeeID-Year-Week"
    total_minutes INT NOT NULL DEFAULT 0
);
