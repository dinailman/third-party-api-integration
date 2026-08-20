INSERT INTO providers (slug,name,category,mapping_rules) VALUES
('provider-a','PulseBand Health','wearables','{"user_id":"user_id","metric_type":"metric_type","value":"value","unit":"unit","timestamp":"recorded_at"}'),
('provider-b','Wellness Cloud','wellness','{"user_id":"member.id","metric_type":"measurement.name","value":"measurement.amount","unit":"measurement.uom","timestamp":"measured_at"}'),
('provider-c','Lab Results Co','diagnostics','{"user_id":"subject.user_id","metric_type":"result.code","value":"result.value","unit":"result.unit","timestamp":"result.observed_at"}')
ON CONFLICT (slug) DO NOTHING;
