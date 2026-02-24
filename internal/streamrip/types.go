package streamrip

type Album struct {
	ID         string
	Title      string
	Artist     string
	Year       int
	Source     string
	URL        string
	Quality    string
	TrackCount int
}

type Progress struct {
	Percent         int
	Status          string
	CurrentTrack    string
	BytesDownloaded int64
	TotalBytes      int64
}

type DownloadJob struct {
	ID       string
	Album    Album
	DestPath string
	Source   string
	Retries  int
}
