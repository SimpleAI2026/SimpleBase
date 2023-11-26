<template>
  <section id="flash-messages" v-show="show">
    <div :class="type">
      <span v-html="text"></span>
      <button type="button" class="close" @click="show = false"><span aria-hidden="true">×</span><span
          class="sr-only">Close</span></button>
    </div>
  </section>
</template>

<script>
import { eventBus } from '../main';

export default {
  data: function () {
    return {
      show: false,
      text: "",
      type: "alert alert-danger"
    }
  },
  methods: {
    showNotification: function (text, type) {
      // escape html, allow line breaks only
      this.text = $("<div/>").text(text).html().replace(/\n/g, "<br />")
      this.type = type;
      this.show = true;
    }
  },
  created: function () {
    eventBus.$on('NotificationError', (text) => {
      this.showNotification(text, 'alert alert-danger');
    });
    eventBus.$on('NotificationInfo', (text) => {
      this.showNotification(text, 'alert alert-info');
    });
    eventBus.$on('NotificationSuccess', (text) => {
      this.showNotification(text, 'alert alert-success');
    });
    eventBus.$on('NotificationClear', () => {
      this.show = false;
    });
  }
}
</script>

<style>
#flash-messages {
  z-index: 10000;
  position: fixed;
  top: 0;
  right: 0;
  z-index: 1100;
  padding: 10px;
  width: 400px;
  max-width: 70%;
  pointer-events: none;
}

#flash-messages .alert {
  transition: all .3s linear;
  pointer-events: auto;
  position: relative;
  padding: 10px 20px;
}

#flash-messages .alert .close {
  font-size: 25px;
  vertical-align: top;
  position: absolute;
  top: 5px;
  right: 10px;
}
</style>
