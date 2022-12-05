<template>
	<div class="page-container page-guide">
		<el-row>
			<el-col :xs="24" :sm="5">
				<doc-side-nav :data="navsData" :base="`/${lang}/guide`"></doc-side-nav>
			</el-col>
			<el-col :xs="24" :sm="19">
				<router-view class="content">111111111111</router-view>
			</el-col>
		</el-row>
	</div>
</template>
<script>
import { default as DocSideNav } from '../../components/DocSideNav'
import navsData from './nav.config.json';

export default {
	components: {
		DocSideNav
	},
	data() {
		return {
			lang: this.$route.meta.lang,
			navsData: navsData["zh-CN"],
		};
	}
};
</script>

<style lang="scss">
.page-guide {
	padding: 25px 50px;
	box-sizing: border-box;

	.content {
		padding-left: 25px;
		margin-left: -1px;

		h2 {
			margin-bottom: 10px;
		}

		h3 {
			font-size: 22px;
			font-weight: normal;
			margin: 0 0 30px;
			color: #1f2d3d;
		}

		p {
			line-height: 1.6;
			font-size: 14px;
			color: #5e6d82;
		}

		ul {
			margin-bottom: 50px;
			padding-left: 0;
		}

		li {
			font-size: 14px;
			margin-bottom: 10px;
			color: #99a9bf;
			list-style: none;

			&:before {
				content: '';
				display: inline-block;
				width: 4px;
				height: 4px;
				border-radius: 50%;
				vertical-align: middle;
				background-color: #5e6d82;
				margin-right: 5px;
			}

			strong {
				color: #5e6d82;
				font-weight: 400;
			}
		}
	}
}

@media (max-width: 768px) {
	.page-guide {
		.content {
			padding-left: 0;
		}
	}
}
</style>