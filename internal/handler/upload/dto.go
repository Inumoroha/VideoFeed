package upload

// uploadResponse 描述上传成功后返回的文件地址、类型、名称和大小。
type uploadResponse struct {
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}
