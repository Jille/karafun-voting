import { WebSocketServer } from 'ws';
import io from "socket.io-client";

const wss = new WebSocketServer({ port: 8067 });

wss.on('connection', function connection(ws) {
	let authed = false;

	const kfsock = io("wss://www.karafun.co.uk/", {
		reconnectionDelayMax: 10000,
		transports: ["websocket"],
		query: {
			"remote": "kf347021"
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
					state: 3,
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
			// TODO: s.singer
			// TODO: s.status (lege string?)
			items.push({
				id: ''+s['id'],
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
		ws.emit('serverUnreacheable');
	});

	ws.on('message', function(data) {
		console.log('received: %s', data);
	});

	kfsock.emit("authenticate", {"login":"proxy","channel":"347021","role":"participant","app":"karafun","socket_id":null});
});
