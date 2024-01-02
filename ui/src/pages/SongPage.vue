<template>
	<q-spinner v-if="loading" />
  <q-page v-else class="row items-center justify-evenly">
		<img :src="song.img" />
    <h1>{{song.artist.name}} - {{song.name}}</h1>
		<div class="subcaption">{{song.rights}}<br>Year {{song.year}}</div>
		{{duration}}
		<ul>
			<li v-for="st in song.styles" :key="st.id"><router-link :to="'/songs/st_' + st.id">{{st.name}}</router-link></li>
		</ul>
		<q-btn label="Lyrics" @click="showLyrics = true" />

		<div v-if="false">
			<h2>Other songs from {{song.artist.name}}</h2>
			<SongsList :filter="'ar_' + song.artist.id +'_'+ song.id" />
		</div>

		<q-dialog v-model="showLyrics">
			<q-card class="q-px-xl">
				<q-card-section style="white-space: pre-line">
					{{song.lyrics}}
				</q-card-section>
			</q-card>
		</q-dialog>
  </q-page>
</template>

<script lang="ts">
import { defineComponent } from 'vue';
import SongsList from '../components/SongsList.vue';

export default defineComponent({
  name: 'SongsPage',
	components: { SongsList },
	props: {
		id: {
			type: String,
			required: true,
		},
	},
	data() {
		return {
			song: {duration: 0},
			loading: true,
			showLyrics: false,
		};
	},
	computed: {
		duration() {
			let secs = this.song.duration % 60 as string | number;
			if(secs < 10) {
				secs = '0' + secs;
			}
			return Math.round(this.song.duration / 60) +':'+ secs;
		}
	},
	watch: {
		id: {
			async handler(id: string) {
				this.loading = true;
				const resp = await this.$axios.get('https://www.karafun.co.uk/347021?type=song_info&id='+ id);
				this.song = resp.data;
				this.loading = false;
			},
			immediate: true,
		},
	},
});
</script>
