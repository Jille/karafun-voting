<template>
  <q-page class="row items-center justify-evenly">
    <SongsList :filter="filter" />
  </q-page>
</template>

<script lang="ts">
import { defineComponent } from 'vue';
import SongsList from '../components/SongsList.vue';

export default defineComponent({
  name: 'SongsPage',
	components: {SongsList},
	props: {
		filter: {
			type: String,
			required: true,
		},
	},
});
</script>
