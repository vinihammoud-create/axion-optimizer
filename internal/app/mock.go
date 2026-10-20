package app

import (
	"time"

	"axion/internal/games"
)

// mockGames feeds the interface preview (mock platform only).
func mockGames() []games.Game {
	lp := time.Now().Add(-5 * time.Hour)
	// An old Steam game (last played 400 days ago, 24 GB): the demo's unused list.
	old := time.Now().Add(-400 * 24 * time.Hour)
	return []games.Game{
		{ID: "steam:730", Name: "Counter-Strike 2", Launcher: "Steam", AppID: "730", InstallDir: `D:\SteamLibrary\steamapps\common\Counter-Strike Global Offensive`,
			Exe: `D:\SteamLibrary\steamapps\common\Counter-Strike Global Offensive\game\bin\win64\cs2.exe`, ExeName: "cs2.exe", ExeSource: "known", Competitive: true, AntiCheat: "VAC", LastPlayed: lp},
		{ID: "riot:valorant.live", Name: "VALORANT", Launcher: "Riot", InstallDir: `C:\Riot Games\VALORANT\live`,
			Exe: `C:\Riot Games\VALORANT\live\ShooterGame\Binaries\Win64\VALORANT-Win64-Shipping.exe`, ExeName: "VALORANT-Win64-Shipping.exe", ExeSource: "known", Competitive: true, AntiCheat: "Vanguard"},
		{ID: "epic:fortnite", Name: "Fortnite", Launcher: "Epic", InstallDir: `C:\Program Files\Epic Games\Fortnite`,
			Exe: `C:\Program Files\Epic Games\Fortnite\FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe`, ExeName: "FortniteClient-Win64-Shipping.exe", ExeSource: "known", Competitive: true, AntiCheat: "EasyAntiCheat/BattlEye"},
		{ID: "steam:1245620", Name: "ELDEN RING", Launcher: "Steam", AppID: "1245620", InstallDir: `D:\SteamLibrary\steamapps\common\ELDEN RING`,
			Exe: `D:\SteamLibrary\steamapps\common\ELDEN RING\Game\eldenring.exe`, ExeName: "eldenring.exe", ExeSource: "known"},
		{ID: "steam:1091500", Name: "Cyberpunk 2077", Launcher: "Steam", AppID: "1091500", InstallDir: `D:\SteamLibrary\steamapps\common\Cyberpunk 2077`,
			Exe: `D:\SteamLibrary\steamapps\common\Cyberpunk 2077\bin\x64\Cyberpunk2077.exe`, ExeName: "Cyberpunk2077.exe", ExeSource: "known",
			LastPlayed: old, Updated: old, SizeBytes: 24 << 30},
	}
}

func mockCS2(list []games.Game) games.CS2Info {
	info := games.CS2Info{LaunchOptions: "-novid -threads 8 +fps_max 0"}
	for i := range list {
		if list[i].ID == "steam:730" {
			g := list[i]
			info.Game, info.Installed = &g, true
		}
	}
	info.Video = &games.CS2Video{Path: `C:\Program Files (x86)\Steam\userdata\123\730\local\cfg\cs2_video.txt`, Modified: time.Now().Add(-48 * time.Hour),
		Width: 1280, Height: 960, RefreshHz: 60, Fullscreen: true, VSync: 0, LowLatency: 0,
		// The video options as the real file of the desktop report had them
		// (MSAA 2x, high shadows with all dynamic shadows, the rest low).
		Raw: map[string]string{"setting.msaa_samples": "2", "setting.r_csgo_cmaa_enable": "0", "setting.videocfg_shadow_quality": "2", "setting.videocfg_dynamic_shadows": "1",
			"setting.videocfg_ao_detail": "0", "setting.videocfg_particle_detail": "0", "setting.videocfg_texture_detail": "0", "setting.r_texturefilteringquality": "0",
			"setting.shaderquality": "0", "setting.videocfg_fsr_detail": "0"}}
	// The Steam client allowing downloads during the game, no limit, no window.
	info.Steam = &games.SteamClient{Found: true, Path: `C:\Program Files (x86)\Steam\config\config.vdf`, DownloadsDuringGameplay: 1, ThrottleKbps: -1, AutoUpdateWindow: 0, WindowStart: -1, WindowEnd: -1}
	info.Convars = map[string]string{"cl_net_buffer_ticks": "1", "rate": "786432", "fps_max": "0"}
	info.ConvarFiles = []string{`C:\Program Files (x86)\Steam\userdata\123\730\local\cfg\cs2_user_convars_0_slot0.vcfg`}
	info.LaunchNotes = games.AnalyzeLaunchOptions(info.LaunchOptions)
	return info
}
