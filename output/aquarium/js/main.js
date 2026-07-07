import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { createAquarium } from './aquarium.js';
import { Clownfish } from './fish.js';
import { createDecorations, updateDecorations } from './decorations.js';
import { createBubbles, updateBubbles } from './bubbles.js';

// === 场景初始化 ===
const scene = new THREE.Scene();
scene.background = new THREE.Color(0x0a0a1a);
scene.fog = new THREE.Fog(0x0a0a1a, 30, 80);

const camera = new THREE.PerspectiveCamera(50, window.innerWidth / window.innerHeight, 0.1, 200);
camera.position.set(25, 10, 25);

const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
renderer.toneMapping = THREE.ACESFilmicToneMapping;
renderer.toneMappingExposure = 1.0;
document.getElementById('canvas-container').appendChild(renderer.domElement);

// === 鼠标旋转视角 ===
const controls = new OrbitControls(camera, renderer.domElement);
controls.enableDamping = true;
controls.dampingFactor = 0.05;
controls.minDistance = 15;
controls.maxDistance = 60;
controls.target.set(0, 0, 0);

// === 灯光 ===
const ambientLight = new THREE.AmbientLight(0x404060, 0.6);
scene.add(ambientLight);

const dirLight = new THREE.DirectionalLight(0xffffff, 0.8);
dirLight.position.set(10, 20, 10);
scene.add(dirLight);

const pointLight1 = new THREE.PointLight(0x4488ff, 0.5, 50);
pointLight1.position.set(-10, 5, 10);
scene.add(pointLight1);

const pointLight2 = new THREE.PointLight(0x00aaff, 0.3, 40);
pointLight2.position.set(10, -5, -10);
scene.add(pointLight2);

// === 鱼缸 ===
const aquarium = createAquarium(20, 12, 14);
scene.add(aquarium);
const bounds = aquarium.userData.bounds;

// === 装饰 ===
const decorations = createDecorations(bounds);
scene.add(decorations);

// === 气泡 ===
const bubbles = createBubbles(bounds, 60);
scene.add(bubbles);

// === 小丑鱼（至少2条） ===
const fishes = [];
fishes.push(new Clownfish(scene, bounds, 0xff6600));
fishes.push(new Clownfish(scene, bounds, 0xff5500));
fishes.push(new Clownfish(scene, bounds, 0xff7733));

// === 点击交互（Raycaster） ===
const raycaster = new THREE.Raycaster();
const mouse = new THREE.Vector2();
const clickableMeshes = [];
fishes.forEach(fish => {
  clickableMeshes.push(...fish.interactiveMeshes);
});

renderer.domElement.addEventListener('click', (event) => {
  mouse.x = (event.clientX / window.innerWidth) * 2 - 1;
  mouse.y = -(event.clientY / window.innerHeight) * 2 + 1;

  raycaster.setFromCamera(mouse, camera);
  const intersects = raycaster.intersectObjects(clickableMeshes);

  if (intersects.length > 0) {
    const fish = intersects[0].object.userData.fish;
    if (fish) {
      fish.scare();
    }
  }
});

// === 窗口适配 ===
window.addEventListener('resize', () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
});

// === 渲染循环 ===
function animate() {
  requestAnimationFrame(animate);

  const time = performance.now();

  fishes.forEach(fish => fish.update());
  updateDecorations(decorations, time);
  updateBubbles(bubbles, time);
  controls.update();

  renderer.render(scene, camera);
}

animate();