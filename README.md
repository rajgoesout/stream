# stream

## Getting Started

### Requirements

- ffmpeg 4.3
- golang 1.13.1

### Running locally

```bash
git clone https://github.com/rajdeepbharati/stream
cd stream
go get github.com/google/uuid
go build .
./stream
```

## Approach

### Single (original) resolution:

- Get mp4 url from user, check its resolution using ffprobe.
- generate a uuid, which is used as the folder name for a particular stream.
- all streams are stored in the `streams` folder in the server.
- ffmpeg commands are invoked from golang. (It's better to use some bindings, but I didn't use it right now due to time constraints)
- If the stream is successfully created, the id/url is sent back to the user.

### Adaptive bitrate (360p,480p,720p,1080p):

- Didn't check resolution of original file, and followed the same steps as above.

## Demo & Deployment

**server deployment**: http://161.35.56.22/streams/

**demo video**: https://youtu.be/zQEBMOA1bQU

**sample stream**: http://161.35.56.22/streams/f4a88d14-f5b9-43c1-b684-d346da76a93d/f4a88d14-f5b9-43c1-b684-d346da76a93d.m3u8

**ABR streams**:

- http://161.35.56.22/streamsabr/e2466dff-05ba-41fd-8b4a-66e0d747954e/360p.m3u8
- http://161.35.56.22/streamsabr/e2466dff-05ba-41fd-8b4a-66e0d747954e/480p.m3u8
- http://161.35.56.22/streamsabr/e2466dff-05ba-41fd-8b4a-66e0d747954e/720p.m3u8
- http://161.35.56.22/streamsabr/e2466dff-05ba-41fd-8b4a-66e0d747954e/1080p.m3u8

## API Endpoints

### `POST /submit/`

Example:

```bash
curl http://161.35.56.22/submit/ --data "input=https://file-examples-com.github.io/uploads/2017/04/file_example_MP4_1280_10MG.mp4"
```

Here, `input` is a url to an mp4 file.
The file is split into chunks of ~10 seconds each, and made streamable.

Sample response:

```json
{
  "Id": "2cfe0e40-61f9-4287-b136-4fab49b597e3",
  "Url": "http://161.35.56.22/streams/2cfe0e40-61f9-4287-b136-4fab49b597e3/2cfe0e40-61f9-4287-b136-4fab49b597e3.m3u8"
}
```

### `GET /streams/<id>/<id>.m3u8`

Use VLC media player or safari browser to stream:

Sample url: http://161.35.56.22/streams/f4a88d14-f5b9-43c1-b684-d346da76a93d/f4a88d14-f5b9-43c1-b684-d346da76a93d.m3u8

### `POST /submitabr/`

```bash
curl http://localhost:80/submitabr/ --data "input=https://file-examples-com.github.io/uploads/2017/04/file_example_MP4_1280_10MG.mp4"
```

Sample response:

```json
{
  "Id": "38a287de-bcbc-44d4-84d0-731188de1630",
  "Url360p": "http://localhost/streamsabr/38a287de-bcbc-44d4-84d0-731188de1630/360p.m3u8",
  "Url480p": "http://localhost/streamsabr/38a287de-bcbc-44d4-84d0-731188de1630/480p.m3u8",
  "Url720p": "http://localhost/streamsabr/38a287de-bcbc-44d4-84d0-731188de1630/720p.m3u8",
  "Url1080p": "http://localhost/streamsabr/38a287de-bcbc-44d4-84d0-731188de1630/1080p.m3u8"
}
```

### `GET /streamsabr/<id>/<resolution>.m3u8`

Use VLC media player or safari browser to stream:

Sample url: http://161.35.56.22/streamsabr/e2466dff-05ba-41fd-8b4a-66e0d747954e/1080p.m3u8
