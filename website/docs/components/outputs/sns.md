---
title: sns
type: output
---

```yaml
sns:
  credentials:
    id: ""
    profile: ""
    role: ""
    role_external_id: ""
    secret: ""
    token: ""
  endpoint: ""
  max_in_flight: 1
  region: eu-west-1
  timeout: 5s
  topic_arn: ""
```

Sends messages to an AWS SNS topic.

### Credentials

By default Benthos will use a shared credentials file when connecting to AWS
services. It's also possible to set them explicitly at the component level,
allowing you to transfer data across accounts. You can find out more
[in this document](../aws.md).
