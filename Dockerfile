#####
# This is a working example of setting up tesseract/gosseract,
# and also works as an example runtime to use gosseract package.
# You can just hit `docker run -it --rm otiai10/gosseract`
# to try and check it out!
#####
FROM golang:1.21-bullseye as builder

WORKDIR /app
# RUN apt-get update -qq

# # You need librariy files and headers of tesseract and leptonica.
# # When you miss these or LD_LIBRARY_PATH is not set to them,
# # you would face an error: "tesseract/baseapi.h: No such file or directory"
# RUN apt-get install -y -qq libtesseract-dev libleptonica-dev

# # In case you face TESSDATA_PREFIX error, you minght need to set env vars
# # to specify the directory where "tessdata" is located.
# ENV TESSDATA_PREFIX=/usr/share/tesseract-ocr/5/tessdata/

# # Load languages.
# # These {lang}.traineddata would b located under ${TESSDATA_PREFIX}/tessdata.
# RUN apt-get install -y -qq \
#   tesseract-ocr-eng \
#   tesseract-ocr-deu \
#   tesseract-ocr-jpn
# See https://github.com/tesseract-ocr/tessdata for the list of available languages.
# If you want to download these traineddata via `wget`, don't forget to locate
# downloaded traineddata under ${TESSDATA_PREFIX}/tessdata.
# Setup your cool project with go.mod.
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target=/root/.cache/go-build go build -o workout_bot src/cmd/main.go
# RUN rm -f /etc/apt/apt.conf.d/docker-clean; echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache
# RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
#   --mount=type=cache,target=/var/lib/apt,sharing=locked \
#   apt update && apt-get --no-install-recommends install -y gcc

FROM golang:1.21-bullseye
RUN mkdir /app
WORKDIR /app

# Copy APT cache from builder stage
# COPY --from=builder /var/cache/apt /var/cache/apt
# COPY --from=builder /var/lib/apt /var/lib/apt

# Install runtime dependencies
RUN apt-get update -qq

# You need librariy files and headers of tesseract and leptonica.
# When you miss these or LD_LIBRARY_PATH is not set to them,
# you would face an error: "tesseract/baseapi.h: No such file or directory"
RUN apt-get install -y -qq libtesseract-dev libleptonica-dev

# In case you face TESSDATA_PREFIX error, you minght need to set env vars
# to specify the directory where "tessdata" is located.
ENV TESSDATA_PREFIX=/usr/share/tesseract-ocr/5/tessdata/

# Load languages.
# These {lang}.traineddata would b located under ${TESSDATA_PREFIX}/tessdata.
RUN apt-get install -y -qq \
  tesseract-ocr-eng \
  tesseract-ocr-deu \
  tesseract-ocr-jpn

COPY --from=builder /app/workout_bot .

CMD ["./workout_bot"]
# Let's have gosseract in your project and test it.
# RUN go get -t github.com/otiai10/gosseract/v2

# Now, you've got complete environment to play with "gosseract"!
# For other OS, check https://github.com/otiai10/gosseract/tree/main/test/runtimes

# Try `docker run -it --rm otiai10/gosseract` to test this environment.

# CMD go test -v github.com/otiai10/gosseract/v2