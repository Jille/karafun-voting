<template>
  <q-page class="row items-center justify-evenly">
		QueuePage
  </q-page>
</template>

<script lang="ts">
import { defineComponent } from 'vue';

export default defineComponent({
  name: 'QueuePage',
	data() {
		return {
			loading: true,
		};
	},
	mounted() {
		const socket = new WebSocket('ws://' + window.location.hostname +':8067/ws');
		socket.addEventListener('open', function() {
			socket.send('{"command": "hello"}');
		});
		socket.addEventListener('message', (event) => {
			console.log('Message from server ', event.data);
		});
	},
});
</script>
