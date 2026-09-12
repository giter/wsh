package server

import "encoding/json"

// handler is a single RPC method: it receives the server, the calling client,
// and raw JSON params, returning a result (or an error serialized as {ok:false}).
type handler func(s *Server, c *wsClient, params json.RawMessage) (interface{}, error)

// router maps method names to handlers.
var router = map[string]handler{
	// connections
	"connections.list":   (*Server).handleListConnections,
	"connections.save":   (*Server).handleSaveConnection,
	"connections.delete": (*Server).handleDeleteConnection,
	"connections.test":   (*Server).handleTestConnection,

	// folders
	"folders.list":   (*Server).handleListFolders,
	"folders.save":   (*Server).handleSaveFolder,
	"folders.delete": (*Server).handleDeleteFolder,

	// settings
	"settings.get":  (*Server).handleGetSettings,
	"settings.save": (*Server).handleSaveSettings,

	// terminals
	"terminal.open":   (*Server).handleOpenTerminal,
	"terminal.input":  (*Server).handleTerminalInput,
	"terminal.resize": (*Server).handleTerminalResize,
	"terminal.close":  (*Server).handleTerminalClose,

	// zmodem (lrzsz) — chunked upload (sendBegin/sendChunk/sendEnd)
	"zmodem.sendBegin": (*Server).handleZmodemSendBegin,
	"zmodem.sendChunk": (*Server).handleZmodemSendChunk,
	"zmodem.sendEnd":   (*Server).handleZmodemSendEnd,
	"zmodem.cancel":    (*Server).handleZmodemCancel,

	// sftp
	"fs.list":       (*Server).handleFsList,
	"sftp.list":     (*Server).handleSftpList,
	"sftp.upload":   (*Server).handleSftpUpload,
	"sftp.download": (*Server).handleSftpDownload,
	"sftp.mkdir":    (*Server).handleSftpMkdir,

	// tunnels
	"tunnels.list":   (*Server).handleListTunnels,
	"tunnels.save":   (*Server).handleSaveTunnel,
	"tunnels.delete": (*Server).handleDeleteTunnel,
	"tunnels.start":  (*Server).handleStartTunnel,
	"tunnels.stop":   (*Server).handleStopTunnel,
}
