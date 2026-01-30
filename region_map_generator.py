"""
Region Map Generator
都市のRegion配置を地図上に可視化
"""

import io
import asyncio
import aiohttp
from PIL import Image, ImageDraw, ImageFont
from typing import List, Dict, Tuple, Optional
import logging

TILE_SIZE = 256  # OpenStreetMapタイルサイズ
ZOOM = 11  # Wplace uses zoom 11


class RegionMapGenerator:
    """Region配置マップを生成"""
    
    def __init__(self, user_agent: str = "WplaceDiscordBot/2.3.1"):
        self.user_agent = user_agent
        self.tile_cache = {}
    
    async def get_osm_tile_async(self, session: aiohttp.ClientSession, x: int, y: int, zoom: int = ZOOM) -> Optional[Image.Image]:
        """OSM HOT Tilesを非同期取得（OSM France運営、安定動作）"""
        cache_key = f"{zoom}/{x}/{y}"
        
        if cache_key in self.tile_cache:
            return self.tile_cache[cache_key]
        
        # OSM France HOT (Humanitarian OpenStreetMap Team)
        # サブドメインランダム選択で負荷分散
        subdomains = ['a', 'b', 'c']
        import random
        subdomain = random.choice(subdomains)
        url = f"https://{subdomain}.tile.openstreetmap.fr/hot/{zoom}/{x}/{y}.png"
        
        try:
            async with session.get(url, timeout=aiohttp.ClientTimeout(total=10)) as response:
                if response.status == 200:
                    tile_data = await response.read()
                    tile = Image.open(io.BytesIO(tile_data))
                    self.tile_cache[cache_key] = tile
                    return tile
                else:
                    logging.warning(f"Failed to fetch tile {url}: {response.status}")
                    return None
        except Exception as e:
            logging.error(f"Error fetching tile {url}: {e}")
            return None
    
    def calculate_region_bounds(self, regions: Dict[str, Dict]) -> Tuple[int, int, int, int]:
        """
        Regionリストから範囲を計算
        
        Returns:
            (min_region_x, max_region_x, min_region_y, max_region_y)
        """
        region_coords = [info['region_coords'] for info in regions.values()]
        
        xs = [coord[0] for coord in region_coords]
        ys = [coord[1] for coord in region_coords]
        
        return min(xs), max(xs), min(ys), max(ys)
    
    async def generate_city_region_map_async(
        self,
        city_name: str,
        regions: Dict[str, Dict],
        highlight_region: Optional[str] = None
    ) -> io.BytesIO:
        """
        都市のRegionマップを非同期生成（高速化版）
        
        Args:
            city_name: 都市名 (例: "Akita")
            regions: Region情報辞書 {"Akita#6": {...}, "Akita#7": {...}, ...}
            highlight_region: ハイライトするRegion名 (例: "Akita#11")
        
        Returns:
            PNG画像のBytesIO
        """
        # Region範囲を計算
        min_rx, max_rx, min_ry, max_ry = self.calculate_region_bounds(regions)
        
        # タイル範囲を計算（Region × 4 = Tile）
        min_tx = min_rx * 4
        max_tx = (max_rx + 1) * 4 - 1
        min_ty = min_ry * 4
        max_ty = (max_ry + 1) * 4 - 1
        
        tile_width = max_tx - min_tx + 1
        tile_height = max_ty - min_ty + 1
        
        logging.info(f"Generating map for {city_name}: tiles {tile_width}×{tile_height}")
        
        # 制限チェック（大きすぎる場合は簡易版）
        # 簡易版は低解像度マップを使用するため、サイズを抑えつつ地図も表示可能
        if tile_width > 32 or tile_height > 32:
            logging.warning(f"Map too large: {tile_width}×{tile_height}, using simplified view")
            return await self._generate_simple_map_async(city_name, regions, highlight_region)
        
        # 背景地図を作成
        map_width = tile_width * TILE_SIZE
        map_height = tile_height * TILE_SIZE
        base_map = Image.new('RGB', (map_width, map_height), color='#E8E8E8')
        
        # タイルを並列ダウンロード（高速化の核心！）
        async with aiohttp.ClientSession(headers={'User-Agent': self.user_agent}) as session:
            tasks = []
            tile_positions = []
            
            for ty in range(min_ty, max_ty + 1):
                for tx in range(min_tx, max_tx + 1):
                    tasks.append(self.get_osm_tile_async(session, tx, ty))
                    tile_positions.append((tx, ty))
            
            # 全タイルを並列取得
            tiles = await asyncio.gather(*tasks)
            
            # タイルを配置
            for (tx, ty), tile in zip(tile_positions, tiles):
                if tile:
                    x_offset = (tx - min_tx) * TILE_SIZE
                    y_offset = (ty - min_ty) * TILE_SIZE
                    base_map.paste(tile, (x_offset, y_offset))
        
        # Region境界線と番号を描画
        draw = ImageDraw.Draw(base_map, 'RGBA')
        
        try:
            font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", 48)
            small_font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 24)
        except:
            font = ImageFont.load_default()
            small_font = font
        
        for region_name, info in regions.items():
            rx, ry = info['region_coords']
            
            region_num = region_name.split('#')[1] if '#' in region_name else '?'
            
            x1 = (rx * 4 - min_tx) * TILE_SIZE
            y1 = (ry * 4 - min_ty) * TILE_SIZE
            x2 = x1 + (4 * TILE_SIZE)
            y2 = y1 + (4 * TILE_SIZE)
            
            is_highlighted = (region_name == highlight_region)
            
            if is_highlighted:
                overlay_color = (255, 215, 0, 100)
                border_color = (255, 165, 0, 255)
                border_width = 8
            else:
                overlay_color = (100, 149, 237, 60)
                border_color = (70, 130, 220, 200)
                border_width = 4
            
            draw.rectangle([x1, y1, x2, y2], fill=overlay_color, outline=border_color, width=border_width)
            
            text = f"#{region_num}"
            
            try:
                bbox = draw.textbbox((0, 0), text, font=font)
                text_width = bbox[2] - bbox[0]
                text_height = bbox[3] - bbox[1]
            except:
                text_width, text_height = draw.textsize(text, font=font)
            
            text_x = x1 + (x2 - x1 - text_width) // 2
            text_y = y1 + (y2 - y1 - text_height) // 2
            
            # テキスト縁取り（簡略化）
            for offset_x in [-2, 0, 2]:
                for offset_y in [-2, 0, 2]:
                    if offset_x != 0 or offset_y != 0:
                        draw.text(
                            (text_x + offset_x, text_y + offset_y),
                            text,
                            fill=(255, 255, 255, 255),
                            font=font
                        )
            
            text_color = (255, 140, 0) if is_highlighted else (0, 0, 139)
            draw.text((text_x, text_y), text, fill=text_color, font=font)
        
        # タイトルを追加
        title_height = 80
        final_image = Image.new('RGB', (map_width, map_height + title_height), color='#2C3E50')
        final_image.paste(base_map, (0, title_height))
        
        draw = ImageDraw.Draw(final_image)
        title_text = f"🗾 {city_name} Region Map ({len(regions)} regions)"
        
        try:
            title_bbox = draw.textbbox((0, 0), title_text, font=small_font)
            title_width = title_bbox[2] - title_bbox[0]
        except:
            title_width, _ = draw.textsize(title_text, font=small_font)
        
        draw.text(
            ((map_width - title_width) // 2, 25),
            title_text,
            fill=(255, 255, 255),
            font=small_font
        )
        
        # BytesIOに保存（圧縮軽量化）
        output = io.BytesIO()
        final_image.save(output, format='PNG', optimize=False)  # optimize=False で高速化
        output.seek(0)
        
        return output
    
    def generate_city_region_map(
        self,
        city_name: str,
        regions: Dict[str, Dict],
        highlight_region: Optional[str] = None
    ) -> io.BytesIO:
        """
        同期版ラッパー（互換性のため）
        """
        return asyncio.run(self.generate_city_region_map_async(city_name, regions, highlight_region))
    
    def _generate_simple_map(
        self,
        city_name: str,
        regions: Dict[str, Dict],
        highlight_region: Optional[str] = None
    ) -> io.BytesIO:
        """
        簡易版マップ生成（同期ラッパー）
        """
        return asyncio.run(self._generate_simple_map_async(city_name, regions, highlight_region))
    
    async def _generate_simple_map_async(
        self,
        city_name: str,
        regions: Dict[str, Dict],
        highlight_region: Optional[str] = None
    ) -> io.BytesIO:
        """
        簡易版マップ生成（低解像度地図付き）
        大きな範囲でもファイルサイズを抑えつつ地図を表示
        """
        min_rx, max_rx, min_ry, max_ry = self.calculate_region_bounds(regions)
        
        # Zoom 7を使用（1 region = 1/4 tile）より低解像度で軽量化
        zoom = 7
        scale = 4  # 1 region = zoom7の1/4タイル
        
        # タイル範囲計算（zoom7ベース）
        min_tx = min_rx // scale
        max_tx = max_rx // scale
        min_ty = min_ry // scale
        max_ty = max_ry // scale
        
        tiles_x = max_tx - min_tx + 1
        tiles_y = max_ty - min_ty + 1
        
        logging.info(f"Simplified map for {city_name}: using zoom {zoom}, tiles {tiles_x}×{tiles_y}")
        
        # 地図タイルを非同期取得（低解像度なので軽量）
        async with aiohttp.ClientSession(headers={'User-Agent': 'Mozilla/5.0'}) as session:
            tasks = []
            for ty in range(tiles_y):
                for tx in range(tiles_x):
                    tile_x = min_tx + tx
                    tile_y = min_ty + ty
                    tasks.append(self.get_osm_tile_async(session, tile_x, tile_y, zoom))
            
            tile_images = await asyncio.gather(*tasks)
        
        # タイル結合
        tile_size = 256
        map_width = tiles_x * tile_size
        map_height = tiles_y * tile_size
        
        base_map = Image.new('RGBA', (map_width, map_height), (240, 240, 240, 255))
        
        idx = 0
        for ty in range(tiles_y):
            for tx in range(tiles_x):
                tile_img = tile_images[idx]
                idx += 1
                
                if tile_img:
                    x = tx * tile_size
                    y = ty * tile_size
                    base_map.paste(tile_img, (x, y))
        
        # Regionグリッドをオーバーレイ
        overlay = Image.new('RGBA', (map_width, map_height), (0, 0, 0, 0))
        draw = ImageDraw.Draw(overlay)
        
        try:
            font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", 18)
            small_font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 14)
        except:
            font = ImageFont.load_default()
            small_font = font
        
        # Region単位のセルサイズ（zoom7では1 region = 64px）
        cell_size = tile_size // scale  # 256 / 4 = 64px
        
        for region_name, info in regions.items():
            rx, ry = info['region_coords']
            region_num = region_name.split('#')[1] if '#' in region_name else '?'
            
            # グリッド位置
            col = rx - min_rx
            row = ry - min_ry
            
            x1 = col * cell_size
            y1 = row * cell_size
            x2 = x1 + cell_size
            y2 = y1 + cell_size
            
            is_highlighted = (region_name == highlight_region)
            
            # 半透明の枠線
            border_color = (255, 140, 0, 200) if is_highlighted else (70, 130, 220, 150)
            fill_color = (255, 215, 0, 100) if is_highlighted else (100, 149, 237, 50)
            
            draw.rectangle([x1, y1, x2, y2], fill=fill_color, outline=border_color, width=2)
            
            # Region番号テキスト
            text = f"#{region_num}"
            try:
                bbox = draw.textbbox((0, 0), text, font=small_font)
                text_width = bbox[2] - bbox[0]
                text_height = bbox[3] - bbox[1]
            except:
                text_width, text_height = draw.textsize(text, font=small_font)
            
            text_x = x1 + (cell_size - text_width) // 2
            text_y = y1 + (cell_size - text_height) // 2
            
            # 白い縁取り
            for offset_x in [-1, 0, 1]:
                for offset_y in [-1, 0, 1]:
                    if offset_x != 0 or offset_y != 0:
                        draw.text(
                            (text_x + offset_x, text_y + offset_y),
                            text,
                            fill=(255, 255, 255, 255),
                            font=small_font
                        )
            
            text_color = (255, 100, 0, 255) if is_highlighted else (0, 0, 139, 255)
            draw.text((text_x, text_y), text, fill=text_color, font=small_font)
        
        # オーバーレイを合成
        base_map = Image.alpha_composite(base_map, overlay)
        
        # RGBに変換（ファイルサイズ削減）
        final_map = Image.new('RGB', (map_width, map_height), (255, 255, 255))
        final_map.paste(base_map, (0, 0))
        
        # タイトルを追加
        title_height = 60
        final_image = Image.new('RGB', (map_width, map_height + title_height), color='#2C3E50')
        final_image.paste(final_map, (0, title_height))
        
        draw = ImageDraw.Draw(final_image)
        title_text = f"🗾 {city_name} Region Map ({len(regions)} regions) - Simplified View"
        
        try:
            title_bbox = draw.textbbox((0, 0), title_text, font=small_font)
            title_width = title_bbox[2] - title_bbox[0]
        except:
            title_width, _ = draw.textsize(title_text, font=small_font)
        
        draw.text(
            ((map_width - title_width) // 2, 20),
            title_text,
            fill=(255, 255, 255),
            font=small_font
        )
        
        # BytesIOに保存（JPEG形式で大幅軽量化）
        output = io.BytesIO()
        final_image.save(output, format='JPEG', quality=85, optimize=True)
        output.seek(0)
        
        return output


def search_regions_by_city(database: Dict[str, Dict], city_name: str) -> Dict[str, Dict]:
    """
    都市名でRegionを検索
    
    Args:
        database: Region database
        city_name: 都市名 (例: "Akita", "Tokyo")
    
    Returns:
        一致したRegion情報の辞書
    """
    results = {}
    
    for region_name, info in database.items():
        if region_name.startswith(f"{city_name}#"):
            results[region_name] = info
    
    return results
