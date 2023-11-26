require('expose-loader?$!expose-loader?jQuery!jquery');
require("bootstrap/dist/js/bootstrap.min");

// eslint-disable-next-line no-unused-vars
import _ from 'lodash/core';

import Vue from 'vue';
import VueRouter from 'vue-router';
Vue.use(VueRouter);

import IndexPage from './pages/IndexPage.vue';
import AppPage from './pages/AppPage.vue';

import FnSidebar from './components/FnSidebar.vue';
import FnNotification from './components/FnNotification.vue';
import { defaultErrorHandler, getAuthToken } from './libs/util';
import App from './app.vue';
import "./css/app.css";

export const eventBus = new Vue();

const numXValues = 50;

const router = new VueRouter({
    routes: [
        { path: '/', component: IndexPage },
        { path: '/app/:appid', component: AppPage }
    ]
});

new Vue({
    el: '#app',
    router: router,
    render: h => {
        return h(App)
    },
    data: {
        apps: null,
        stats: 0,
        statshistory: null,
        autorefresh: null
    },
    components: {
        IndexPage,
        FnSidebar,
        FnNotification
    },
    methods: {
        loadApps: function () {
            var t = this;
            $.ajax({
                headers: { 'Authorization': getAuthToken() },
                url: '/api/apps',
                dataType: 'json',
                success: (apps) => t.apps = apps,
                error: defaultErrorHandler
            });
        },
        initialiseStatshistory: function () {
            if (this.statshistory == null) {
                this.statshistory = [];
                for (var i = 0; i < numXValues; i++) {
                    this.statshistory.push({});
                }
            }
        },
        loadStats: function () {
            if (this.autorefresh) {
                $.ajax({
                    url: '/api/stats',
                    dataType: 'json',
                    success: this.handleStats,
                    error: defaultErrorHandler
                });
            } else {
                // refresh the graphs using the cached data
                eventBus.$emit('statsRefreshed');
            }
        },
        handleStats: function (statistics) {
            this.stats = statistics;
            if (this.statshistory == null) {
                this.statshistory = [statistics];
            } else {
                this.statshistory.push(statistics);
                if (this.statshistory.length > numXValues) {
                    this.statshistory.shift();
                }
            }
            // we have new stats: notify any graphs to update themselves
            eventBus.$emit('statsRefreshed');
        }
    },
    created: function () {
        var timer;
        this.autorefresh = true;
        this.initialiseStatshistory();
        this.loadApps();
        this.loadStats();
        eventBus.$on('startAutoRefreshStats', () => {
            this.autorefresh = true;
            // we leave the timer running for ever
            if (timer == null) {
                timer = setInterval(function () {
                    this.loadStats();
                }.bind(this), 1000);
            }
        });
        eventBus.$on('stopAutoRefreshStats', () => {
            this.autorefresh = false;
            // leave the timer running as this is the best way to ensure that the graphs keep displaying the cached data when we switch between apps and the index page
            // loadStats() will check the autorefresh flag and simply refresh the graphs
            // if (timer !=null){
            //   clearInterval(timer);
            //   timer = null;
            // }
        });
        eventBus.$on('AppAdded', () => {
            this.loadApps();
            this.loadStats();
        });
        eventBus.$on('AppUpdated', () => {
            this.loadApps();
            this.loadStats();
        });
        eventBus.$on('AppDeleted', () => {
            this.loadApps();
            this.loadStats();
        });
        eventBus.$on('LoggedIn', () => {
            this.loadApps();
        });
    }
});
