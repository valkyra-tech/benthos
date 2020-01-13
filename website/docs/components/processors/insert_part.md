---
title: insert_part
type: processor
---

```yaml
insert_part:
  content: ""
  index: -1
```

Insert a new message into a batch at an index. If the specified index is greater
than the length of the existing batch it will be appended to the end.

The index can be negative, and if so the message will be inserted from the end
counting backwards starting from -1. E.g. if index = -1 then the new message
will become the last of the batch, if index = -2 then the new message will be
inserted before the last message, and so on. If the negative index is greater
than the length of the existing batch it will be inserted at the beginning.

The new message will have metadata copied from the first pre-existing message of
the batch.

This processor will interpolate functions within the 'content' field, you can
find a list of functions [here](../config_interpolation.md#functions).
