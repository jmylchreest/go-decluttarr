# API contract fixtures

Synthetic, sanitized examples derived from these upstream revisions:

- [Sportarr queue endpoint](https://github.com/Sportarr/Sportarr/blob/9d3ef23ec2d8dcf76187a43a1881977eb12a1d7e/src/Endpoints/SonarrQueueEndpoints.cs)
- [Bookshelf queue resource](https://github.com/pennydreadful/bookshelf/blob/3ac41f7e31427684a37138e8e539b7e6c7b73869/src/Readarr.Api.V1/Queue/QueueResource.cs)

These are not captured production responses. Sportarr includes an event and a
pending import without an event identity. Bookshelf includes decimal byte counts
and deliberately omits the unsupported added timestamp.
