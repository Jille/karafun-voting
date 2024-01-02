import { WebSocketServer } from 'ws';
import io from "socket.io-client";

const wss = new WebSocketServer({ port: 8067 });

wss.on('connection', function connection(ws, request) {
	const channel = request.headers['x-karafun-channel'];
	let authed = false;

	const kfsock = io("wss://www.karafun.co.uk/", {
		reconnectionDelayMax: 10000,
		transports: ["websocket"],
		query: {
			"remote": "kf" + channel,
		}
	});

	kfsock.on('connect', function() {
		console.log('kfsock connect event');
	});

	kfsock.on('permissions', function(perms) {
		if(!authed) {
			ws.send(JSON.stringify({
				type: 'core.AuthenticatedEvent',
				payload: {},
			}));
			authed = true;
		}
		ws.send(JSON.stringify({
			type: 'remote.PermissionsUpdateEvent',
			payload: {
				permissions: {
					"manageQueue": perms.includes('manageQueue'),
					"viewQueue": perms.includes('viewQueue'),
					"addToQueue": perms.includes('addToQueue'),
					"managePlayback": perms.includes('manageKaraoke'),
					"manageVolumes": perms.includes('managePlayer'),
					"sendPhotos": perms.includes('uploadPicture'),
				},
			},
		}));
	});
	kfsock.on('preferences', function(prefs) {
		ws.send(JSON.stringify({
			type: 'remote.PreferencesUpdateEvent',
			payload: {
				preferences: {
					askOptions: prefs.askSingerName,
					// TODO: prefs.generalVolume (bool)
					// TODO: prefs.micVolume (bool)
				},
			},
		}));
	});
	kfsock.on('status', function(status) {
		const tracks = [];
		if('volumeBv' in status) {
			tracks.push({
				track: {type: 4},
				volume: status['volumeBv'],
			});
		}
		if('volumeLd' in status) {
			const lead = Object.entries(status['volumeLd'])[0];
			tracks.push({
				track: {type: 5, caption: lead[0], color: { red: 0, green: 0, blue: 0}},
				volume: lead[1],
			});
		}
		ws.send(JSON.stringify({
			type: 'remote.StatusEvent',
			payload: {
				status: {
					state: status['state'] == 'playing' ? 4 : status['state'] == 'paused' ? 5 : 3,
					tempo: status['tempo'],
					pitch: status['pitch'],
					tracks: tracks,
				},
			},
		}));
	});
	kfsock.on('queue', function(queue) {
		const items = [];
		for(const s of queue) {
			// TODO: s.status (lege string?)
			items.push({
				id: ''+s['id'],
				singer: s['singer'] ?? '',
				song: {
					id: {
						type: 1,
						id: 1337, // Niet in de oude api :(
					},
					"title": s['title'],
					"artist": s['artist'],
					"songTracks":[]
				}
			});
		}
		ws.send(JSON.stringify({
			type: 'remote.QueueEvent',
			payload: {
				queue: {
					items: items,
				},
			},
		}));
	});

	kfsock.on('serverUnreacheable', function() {
		console.log('received serverUnreacheable from kf');
		ws.send(JSON.stringify({
			type: 'ServerUnreachable',
			payload: {},
		}));
	});

	ws.on('message', function(raw) {
		console.log('received: %s', raw);
		const data = JSON.parse(raw);
		switch(data.type) {
			case 'remote.AddToQueueResponse':
				kfsock.emit("queueAdd", {"songId": +data.payload.identifier.id, "pos": +data.payload.position, "singer": data.payload.singer ?? ''});
				break;
			case 'remote.MoveInQueueRequest':
				kfsock.emit("queueMove", {"queueId": +data.payload.queueItemId, "from": +data.payload.queueItemId, "to": +data.payload.to});
				break;
			case 'remote.RemoveFromQueueRequest':
				kfsock.emit("queueRemove", +data.payload.queueItemId);
				break;
			case 'remote.PlayRequest':
				kfsock.emit("play", null);
				break;
			case 'remote.PauseRequest':
				kfsock.emit("pause", null);
				break;
			case 'remote.NextRequest':
				kfsock.emit("next", null);
				break;
			case 'remote.TrackVolumeRequest':
				if(data.payload.type == 4) {
					kfsock.emit("volumeBv", data.payload.volume);
				} else {
					console.log("Changing the volume for type != 4 is not yet implemented")
				}
				break;
			case 'remote.PitchRequest':
				kfsock.emit("pitch", data.payload.pitch);
				break;
			case 'remote.TempoRequest':
				kfsock.emit("tempo", data.payload.tempo);
				break;
			default:
				console.log("Ignoring unknown command ", data.type)
		}
	});

	ws.on('close', function() {
		console.log('Client disconnected, closing connection to karafun');
		kfsock.disconnect()
	});

	kfsock.on('loginAlreadyTaken', function() {
		kfsock.emit("authenticate", {"login":"proxy " + Math.random(),"channel":channel,"role":"participant","app":"karafun","socket_id":null});
	});

	kfsock.emit("authenticate", {"login":"proxy","channel":channel,"role":"participant","app":"karafun","socket_id":null});
});
