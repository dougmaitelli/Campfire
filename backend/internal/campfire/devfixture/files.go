//go:build !production

package devfixture

// ConsoleLogs is the initial output shown by both simulated Spark transports.
const ConsoleLogs = "[12:00:00] [Server thread/INFO]: Starting Minecraft server on *:25565\n" +
	"[12:00:01] [Server thread/INFO]: Loading properties\n" +
	"[12:00:01] [Server thread/INFO]: Default game type: SURVIVAL\n" +
	"[12:00:02] [Server thread/INFO]: Preparing level \"world\"\n" +
	"[12:00:03] [Server thread/INFO]: Preparing spawn area: 100%\n" +
	"[12:00:04] [Server thread/INFO]: Done (3.425s)! For help, type \"help\"\n" +
	"[12:05:00] [Server thread/INFO]: Alex joined the game\n" +
	"[12:05:06] [Server thread/INFO]: Jamie joined the game\n" +
	"[12:10:00] [Server thread/INFO]: Saved the game\n"

// Files returns an independent set of the shared starter game files.
func Files() map[string]string {
	return map[string]string{
		"server.properties":            "motd=Campfire mock server\nserver-port=25565\nmax-players=20\n",
		"eula.txt":                     "# Simulated fixture; no Minecraft software is running.\neula=false\n",
		"whitelist.json":               "[]\n",
		"ops.json":                     "[]\n",
		"config/":                      "",
		"config/fabric/":               "",
		"config/fabric/settings.json":  "{\n  \"debug\": false,\n  \"maxViewDistance\": 12\n}\n",
		"config/permissions.json":      "{\n  \"defaultRole\": \"player\"\n}\n",
		"logs/":                        "",
		"logs/latest.log":              ConsoleLogs,
		"mods/":                        "",
		"mods/README.txt":              "Place Fabric mod JARs here. This mock server does not execute them.\n",
		"world/":                       "",
		"world/datapacks/":             "",
		"world/datapacks/README.txt":   "Place world datapacks here.\n",
		"world/playerdata/":            "",
		"world/stats/":                 "",
		"world/stats/mock-player.json": "{\n  \"stats\": {\"minecraft:custom\": {\"minecraft:play_time\": 72000}},\n  \"DataVersion\": 4189\n}\n",
		"crash-reports/":               "",
	}
}
